package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/fcasset"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

// ErrNotPublishable is returned for a contract that is not approved, signed or
// active.
var ErrNotPublishable = errors.New("only an approved, signed or active contract can be registered in the catalogue")

var publishableStates = []string{contractstate.Approved.String(), contractstate.Signed.String(), contractstate.Active.String()}

type CataloguePublishCmd struct {
	DID         string
	PublishedBy string
	HolderDID   string
	// InstanceDID is this DCS deployment's DID, the issuer of the catalogue
	// entry.
	InstanceDID string
	UserRoles   userrole.UserRoles
}

type CataloguePublisher struct {
	DB       *sqlx.DB
	CRepo    db.ContractRepo
	FCClient *fcclient.FederatedCatalogueClient
}

// catalogueAssets is the part of the Federated Catalogue client the contract
// entries use.
type catalogueAssets interface {
	PublishAsset(ctx context.Context, payload map[string]any) (string, error)
	RevokeAsset(ctx context.Context, assetHash string) error
}

// lockCatalogueEntry serializes the catalogue steps of one contract across
// replicas until the transaction ends.
func lockCatalogueEntry(ctx context.Context, tx *sqlx.Tx, did string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('contract-catalogue:' || $1))`, did); err != nil {
		return fmt.Errorf("could not lock the catalogue entry: %w", err)
	}
	return nil
}

// Handle registers the contract metadata in the Federated Catalogue and
// records the registration in the audit trail (DCS-IR-CWE-10). The catalogue
// entry holds no contract content and no party. A contract has at most one
// entry: a new registration revokes the entry before it. The audit record and
// the stored entry are written in one transaction after the catalogue call; if
// that transaction fails, the new entry is revoked again.
func (h *CataloguePublisher) Handle(ctx context.Context, cmd CataloguePublishCmd) (string, error) {
	if h.FCClient == nil {
		return "", fcclient.ErrFederatedCatalogueNotConfigured
	}
	return h.handle(ctx, h.FCClient, cmd)
}

func (h *CataloguePublisher) handle(ctx context.Context, assets catalogueAssets, cmd CataloguePublishCmd) (string, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	if err := lockCatalogueEntry(ctx, tx, cmd.DID); err != nil {
		return "", err
	}
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return "", fmt.Errorf("could not read contract: %w", err)
	}
	if !slices.Contains(publishableStates, contract.State) {
		return "", ErrNotPublishable
	}
	name := ""
	if contract.Name != nil {
		name = *contract.Name
	}
	payload, err := fcasset.BuildContractPayload(cmd.InstanceDID, base.ResourceIRI("contract", cmd.DID), contract.UpdatedAt, fcasset.ContractSubject{
		ID:         cmd.DID,
		State:      contract.State,
		Name:       name,
		Version:    contract.ContractVersion,
		TemplateID: contract.TemplateDID,
		StartDate:  contract.StartDate,
		EndDate:    contract.ExpDate,
	})
	if err != nil {
		return "", fmt.Errorf("build contract catalogue entry: %w", err)
	}

	var previous string
	if err := tx.GetContext(ctx, &previous, `SELECT asset_hash FROM contract_catalogue_entries WHERE did = $1`, cmd.DID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("could not read the catalogue entry: %w", err)
	}
	if previous != "" {
		if err := withdrawCatalogueEntry(ctx, tx, assets, cmd.DID, previous, contract.State); err != nil {
			return "", err
		}
	}

	assetHash, err := assets.PublishAsset(ctx, payload)
	if err != nil {
		if previous != "" {
			// The revoked entry is gone from the catalogue; keep that record.
			if commitErr := tx.Commit(); commitErr != nil {
				log.Printf("could not record the revoked catalogue entry of %s: %v", cmd.DID, commitErr)
			}
		}
		return "", fmt.Errorf("register contract in the catalogue: %w", err)
	}

	if err := recordCatalogueEntry(ctx, tx, cmd, contract.ContractVersion, assetHash); err != nil {
		revokeUnrecorded(ctx, assets, cmd.DID, assetHash)
		return "", err
	}
	if err := tx.Commit(); err != nil {
		revokeUnrecorded(ctx, assets, cmd.DID, assetHash)
		return "", fmt.Errorf("could not commit the catalogue entry: %w", err)
	}
	return assetHash, nil
}

func recordCatalogueEntry(ctx context.Context, tx *sqlx.Tx, cmd CataloguePublishCmd, version int, assetHash string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO contract_catalogue_entries (did, asset_hash) VALUES ($1, $2)
		ON CONFLICT (did) DO UPDATE SET asset_hash = EXCLUDED.asset_hash, published_at = CURRENT_TIMESTAMP`, cmd.DID, assetHash); err != nil {
		return fmt.Errorf("could not store the catalogue entry: %w", err)
	}
	evt := contractevents.CataloguePublishEvent{
		DID:             cmd.DID,
		HolderDID:       cmd.HolderDID,
		ContractVersion: version,
		AssetHash:       assetHash,
		PublishedBy:     cmd.PublishedBy,
		OccurredAt:      time.Now().UTC(),
		UserRoles:       cmd.UserRoles,
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	return nil
}

// revokeUnrecorded takes back a catalogue entry whose audit record could not
// be written, so the catalogue holds no entry the audit trail does not show.
func revokeUnrecorded(ctx context.Context, assets catalogueAssets, did, assetHash string) {
	if err := assets.RevokeAsset(context.WithoutCancel(ctx), assetHash); err != nil {
		log.Printf("could not revoke the unrecorded catalogue entry %s of %s: %v", assetHash, did, err)
	}
}

// withdrawCatalogueEntry revokes the entry in the catalogue, removes it and
// records the withdrawal in tx.
func withdrawCatalogueEntry(ctx context.Context, tx *sqlx.Tx, assets catalogueAssets, did, assetHash, state string) error {
	if err := assets.RevokeAsset(ctx, assetHash); err != nil {
		return fmt.Errorf("could not withdraw the contract from the catalogue: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contract_catalogue_entries WHERE did = $1`, did); err != nil {
		return fmt.Errorf("could not remove the catalogue entry: %w", err)
	}
	evt := contractevents.CatalogueWithdrawEvent{DID: did, AssetHash: assetHash, State: state, OccurredAt: time.Now().UTC()}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	return nil
}

// CatalogueWithdrawer revokes the catalogue entries of contracts that left
// the published states: terminated, expired, revoked or otherwise no longer
// approved, signed or active (DCS-FR-CWE-23). It covers every path that ends
// a contract, local or from a peer.
type CatalogueWithdrawer struct {
	DB       *sqlx.DB
	FCClient *fcclient.FederatedCatalogueClient
}

// Start sweeps every interval until ctx ends.
func (w *CatalogueWithdrawer) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.Sweep(ctx)
			}
		}
	}()
}

// Sweep withdraws every entry of a contract that is no longer publishable.
// Each entry is handled in its own transaction under the entry lock, so
// replicas that sweep together do not race.
func (w *CatalogueWithdrawer) Sweep(ctx context.Context) {
	if w.FCClient == nil {
		return
	}
	sweepCatalogue(ctx, w.DB, w.FCClient)
}

func sweepCatalogue(ctx context.Context, database *sqlx.DB, assets catalogueAssets) {
	var dids []string
	if err := database.SelectContext(ctx, &dids, `
		SELECT e.did FROM contract_catalogue_entries e
		JOIN contracts_effective c ON c.did = e.did
		WHERE c.state::text <> ALL($1)`, pq.Array(publishableStates)); err != nil {
		log.Printf("could not read catalogue entries to withdraw: %v", err)
		return
	}
	for _, did := range dids {
		if err := withdrawIfUnpublishable(ctx, database, assets, did); err != nil {
			log.Printf("could not withdraw catalogue entry of %s: %v", did, err)
		}
	}
}

func withdrawIfUnpublishable(ctx context.Context, database *sqlx.DB, assets catalogueAssets, did string) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCatalogueEntry(ctx, tx, did); err != nil {
		return err
	}
	var entry struct {
		AssetHash string `db:"asset_hash"`
		State     string `db:"state"`
	}
	err = tx.GetContext(ctx, &entry, `
		SELECT e.asset_hash, c.state::text AS state FROM contract_catalogue_entries e
		JOIN contracts_effective c ON c.did = e.did WHERE e.did = $1`, did)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if slices.Contains(publishableStates, entry.State) {
		return nil
	}
	if err := withdrawCatalogueEntry(ctx, tx, assets, did, entry.AssetHash, entry.State); err != nil {
		return err
	}
	return tx.Commit()
}
