package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/provenance"

	"github.com/jmoiron/sqlx"
)

// PeerPdfReceiveCmd carries the machine-readable JSON-LD extracted from a
// contract PDF that a counterparty shipped (ADR-13). The receiver rebuilds its
// own local copy of the contract from it.
type PeerPdfReceiveCmd struct {
	ContractIRI string
	// Counterparty is the peer that shipped the PDF — the contract's origin,
	// from this instance's perspective.
	Counterparty string
	// LocalPeer is this instance's own DID — the other party and the holder of
	// the local RBAC roles.
	LocalPeer string
	// Payload is the JSON-LD contract document pdf-core extracted from the PDF.
	Payload []byte
	// Pdf is the EXACT contract PDF the peer shipped. It is carried over as this
	// instance's copy (not regenerated), so the counterparty's C2PA provenance
	// chain embedded in it is preserved (ADR-13).
	Pdf []byte
	// AdoptRevoked is set when the authenticated counterparty declared the
	// shipped contract REVOKED: it revoked its own applied signature, which
	// voids the agreement regardless of this instance's own workflow progress
	// (DCS-NFR-BR-06). The sole exception to intrinsic-state privacy.
	AdoptRevoked bool
}

// PeerPdfReceiver upserts a peer-shipped contract into this instance's own
// store and opens its own local workflow tasks (ADR-13): each DCS runs its own
// RBAC; nothing crosses the boundary.
type PeerPdfReceiver struct {
	DB        *sqlx.DB
	CRepo     db.ContractRepo
	RTRepo    db.ReviewTaskRepo
	ATRepo    db.ApprovalTaskRepo
	NTRepo    db.NegotiationTaskRepo
	Artifacts *artifactstore.Store
}

// Handle upserts the local copy from the shipped contract's JSON-LD. A first
// ship creates the copy owned by the counterparty (its origin); a later ship
// updates the content and bumps the local version. The contract lands in
// NEGOTIATION — the settlement and signing phases are separate ships (ADR-13).
func (h *PeerPdfReceiver) Handle(ctx context.Context, cmd PeerPdfReceiveCmd) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Serialize receipts per contract IRI: a peer ships once per render, so two
	// ships of the same contract can be in flight at once. Both would read no
	// local copy yet, both would insert, and the loser dies on the primary key
	// with its payload — the newer document — dropped, leaving this instance
	// holding the older one until a retry lands. Queuing here makes the second
	// receipt see the first one's copy and update it. Released on tx
	// commit/rollback.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", cmd.ContractIRI); err != nil {
		return fmt.Errorf("could not acquire the per-contract receipt lock for %s: %w", cmd.ContractIRI, err)
	}

	existing, err := h.CRepo.ReadProcessDataByDIDOrNil(ctx, tx, cmd.ContractIRI)
	if err != nil {
		return fmt.Errorf("could not read local contract copy: %w", err)
	}

	// pdf-core carries the contract JSON-LD VERBATIM (ADR-13): the shipped payload
	// is already this instance's native DCS form — the exact bytes the originator
	// embedded — so it is used as-is, with no dialect round-trip.
	payload := datatype.JSON(cmd.Payload)
	templateIRI, templateVersion, name := parseShippedContractMeta(cmd.Payload)
	now := time.Now().UTC()

	data := db.Contract{
		DID:             cmd.ContractIRI,
		UpdatedAt:       now,
		ContractData:    &payload,
		TemplateDID:     templateIRI,
		TemplateVersion: templateVersion,
		Name:            name,
	}

	if existing != nil {
		// A re-ship (a counteroffer or a settled/signed version) refreshes the
		// content but must not clobber this instance's own local RBAC progress —
		// its intrinsic state is private and advances through its own workflow.
		// The one exception is a revocation ship (DCS-NFR-BR-06): the
		// counterparty voided the agreement, so the local copy hard-stops in
		// REVOKED no matter where its own workflow stood.
		data.State = existing.State
		if cmd.AdoptRevoked {
			data.State = contractstate.Revoked.String()
		}
		data.Origin = existing.Origin
		data.CreatedBy = existing.CreatedBy
		data.ContractVersion = existing.ContractVersion + 1
		if err := h.CRepo.RemoteUpdate(ctx, tx, data); err != nil {
			return fmt.Errorf("could not update local contract copy: %w", err)
		}
		// A new document arrived, so the round it belongs to is a new one: any
		// engagement this instance had with the superseded version is owed again
		// against this one. This carries tasks forward, it never mints a first
		// one — receiving a document is not engaging with it.
		if err := h.NTRepo.RollForward(ctx, tx, cmd.ContractIRI, existing.ContractVersion, data.ContractVersion); err != nil {
			return fmt.Errorf("could not carry negotiation tasks to the new contract version: %w", err)
		}
	} else {
		// A first receipt is an inbound offer: this instance's intrinsic state
		// starts at OFFERED (an offer on our table, awaiting our own review),
		// which its local review/approval tasks then advance. The peer-facing
		// extrinsic lifecycle (proposed → agreed → executed) is inferred from
		// this plus the shipped PDF. A first receipt that already declares a
		// revocation (this instance never saw the offer) lands directly in
		// REVOKED — there is nothing left to review.
		data.State = contractstate.Offered.String()
		if cmd.AdoptRevoked {
			data.State = contractstate.Revoked.String()
		}

		// The two parties are objective on both copies: the origin (the peer that
		// created and offered the contract) and this instance. This instance's own
		// users hold the local RBAC roles.
		resp := db.Responsible{
			Creator:      cmd.Counterparty,
			Counterparty: cmd.LocalPeer,
			Reviewers:    []string{cmd.LocalPeer},
			Approvers:    []string{cmd.LocalPeer},
			Negotiators:  []string{cmd.LocalPeer},
		}
		data.Origin = cmd.Counterparty
		data.CreatedBy = cmd.Counterparty
		data.CreatedAt = now
		data.ContractVersion = 1
		data.Responsible = &resp
		if err := h.CRepo.RemoteCreate(ctx, tx, data); err != nil {
			return fmt.Errorf("could not create local contract copy: %w", err)
		}
		// Review and approval tasks only. No negotiation task is minted here:
		// receiving an offer is not engaging with it, and a task on arrival would
		// make submit's "no open tasks" gate answer for a round nobody entered.
		// The counterparty mints its own by accepting the offer or by proposing a
		// redline on it (command/acceptoffer.go, command/negotiate.go).
		if err := createReviewAndApprovalTasks(ctx, tx, h.RTRepo, h.ATRepo, cmd.ContractIRI, cmd.LocalPeer, resp); err != nil {
			return err
		}
	}

	// Carry over the EXACT PDF the peer shipped as this instance's copy: the
	// peer's PDF is the authoritative artifact and embeds its own C2PA provenance
	// chain, so regenerating it here would strip the counterparty's C2PA. The
	// service verified the human-readable render matches the embedded JSON-LD
	// before this; this instance's own later changes append to this base (so the
	// C2PA chain grows rather than resetting).
	if len(cmd.Pdf) > 0 {
		// The receiver encrypts the peer's verbatim bytes with its OWN CEK; the
		// inbound PDF stays authoritative and byte-identical after decrypt.
		storedCID, err := h.Artifacts.Put(ctx, artifactstore.ContractScope(cmd.ContractIRI), cmd.Pdf)
		if err != nil {
			return fmt.Errorf("could not store carried-over peer PDF in IPFS: %w", err)
		}
		// pdf_c2pa_state describes the STORED ARTIFACT, not this instance's
		// workflow. The peer ships a PDF it may already have signed while our own
		// copy starts at OFFERED, so mapping the local state alone files a
		// PAdES-signed artifact as "draft": a later local terminate, or the expiry
		// cron, would then see a re-renderable draft and append a C2PA manifest
		// onto the counterparty's signed bytes. The shipped bytes are in hand
		// here, so the artifact answers for itself once, at the moment it is
		// stored, instead of every lifecycle event afterwards having to ask. It is
		// also this instance's record that the agreement is settled, which
		// requireUnsettledAgreement reads to refuse a renegotiation of a document
		// the peer already signed.
		c2paState, err := provenance.ArtifactC2PAState(data.State, cmd.Pdf)
		if err != nil {
			return fmt.Errorf("could not map contract state to C2PA lifecycle: %w", err)
		}
		// Hash the contract_data AS PERSISTED, not the shipped bytes: Postgres
		// normalizes JSONB on write, and the PDF export readiness gate (and the
		// local regenerator) recompute this hash from the stored contract_data.
		// Hashing the shipped bytes leaves the two permanently unequal, so export
		// would wait forever for a regeneration that never runs for a received
		// contract — the carried-over PDF must be servable straight away.
		persisted, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.ContractIRI)
		if err != nil {
			return fmt.Errorf("could not re-read persisted contract data for %s: %w", cmd.ContractIRI, err)
		}
		var persistedData []byte
		if persisted.ContractData != nil {
			persistedData = []byte(*persisted.ContractData)
		}
		payloadSum := sha256.Sum256(persistedData)
		if err := h.CRepo.UpdatePDFState(ctx, tx, cmd.ContractIRI, db.ContractPDFState{
			IPFSCID:     storedCID,
			C2PAState:   c2paState,
			PayloadHash: hex.EncodeToString(payloadSum[:]),
		}); err != nil {
			return fmt.Errorf("could not record carried-over PDF state: %w", err)
		}
	}

	return tx.Commit()
}

// parseShippedContractMeta pulls the template provenance and title out of the
// shipped contract's JSON-LD (derivedFromTemplate.@id ends in the template IRI).
func parseShippedContractMeta(payload []byte) (templateIRI string, templateVersion int, name *string) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return "", 0, nil
	}
	if prov, ok := doc["derivedFromTemplate"].(map[string]any); ok {
		if id, ok := prov["@id"].(string); ok {
			templateIRI = id[strings.LastIndex(id, "/")+1:]
		}
		if v, ok := prov["version"].(float64); ok {
			templateVersion = int(v)
		}
	}
	if meta, ok := doc["dcs:metadata"].(map[string]any); ok {
		if t, ok := meta["dcs:title"].(string); ok && t != "" {
			name = &t
		}
	}
	return templateIRI, templateVersion, name
}
