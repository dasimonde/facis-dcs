package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/middleware"
)

func contractReadContext(ctx context.Context, document identity.DIDDocument) (context.Context, error) {
	peer, err := document.GetID()
	if err != nil {
		return ctx, contractworkflowengine.MakeInternalError(err)
	}
	return contractreadpolicy.WithScope(ctx, contractreadpolicy.Scope{Organization: middleware.GetParticipantID(ctx), Roles: middleware.GetUserRoles(ctx), LocalPeer: peer}), nil
}

type contractAccessRow struct {
	Origin    string         `db:"origin"`
	CreatedBy string         `db:"created_by"`
	Data      *datatype.JSON `db:"contract_data"`
}

func readContractAccessRow(ctx context.Context, database *sqlx.DB, did string) (*contractAccessRow, error) {
	var row contractAccessRow
	err := database.GetContext(ctx, &row, `SELECT COALESCE(origin, '') AS origin, created_by, contract_data FROM contracts_effective WHERE did = $1`, did)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, contractworkflowengine.MakeNotFound(err)
	}
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	return &row, nil
}

// authorizeContractRead checks current trusted metadata before any derived read or read-side event.
func authorizeContractRead(ctx context.Context, database *sqlx.DB, localPeer, did string) error {
	row, err := readContractAccessRow(ctx, database, did)
	if err != nil {
		return err
	}
	scope := contractreadpolicy.Scope{Organization: middleware.GetParticipantID(ctx), Roles: middleware.GetUserRoles(ctx), LocalPeer: localPeer}
	if !scope.MayRead(row.Origin, row.CreatedBy, row.Data) {
		return contractworkflowengine.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	return nil
}

// authorizeContractWrite admits a write by the creating organization, by an
// organization named in dcs:parties, or by a system client that acts for this
// instance (participant = the instance DID). A Sys.* role gives no write
// access to the contracts of other organizations (ADR-27).
func authorizeContractWrite(ctx context.Context, database *sqlx.DB, document identity.DIDDocument, did string) error {
	peer, err := document.GetID()
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	row, err := readContractAccessRow(ctx, database, did)
	if err != nil {
		return err
	}
	organization := middleware.GetParticipantID(ctx)
	if organization != "" && organization == peer {
		return nil
	}
	scope := contractreadpolicy.Scope{Organization: organization, LocalPeer: peer}
	if !scope.MayRead(row.Origin, row.CreatedBy, row.Data) {
		return contractworkflowengine.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	return nil
}

// authorizeContractOwner admits only the creating organization, or a system
// client that acts for this instance on a contract this instance originated.
// A party that did not create the contract is refused.
func authorizeContractOwner(ctx context.Context, database *sqlx.DB, document identity.DIDDocument, did string) error {
	peer, err := document.GetID()
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	row, err := readContractAccessRow(ctx, database, did)
	if err != nil {
		return err
	}
	organization := middleware.GetParticipantID(ctx)
	switch {
	case organization == "":
	case organization == row.CreatedBy:
		return nil
	case organization == peer && (row.Origin == "" || row.Origin == peer):
		return nil
	}
	return contractworkflowengine.MakeForbidden(contractreadpolicy.ErrDenied)
}

// ContractEventAudience admits a webhook subscriber organization to a contract
// event only when it may read the contract. Template events are not
// organization-scoped. An unknown or unreadable contract admits nobody.
func ContractEventAudience(database *sqlx.DB, localPeer string) func(ctx context.Context, event, did, organization string) bool {
	return func(ctx context.Context, event, did, organization string) bool {
		if strings.HasPrefix(event, "template.") {
			return true
		}
		var row struct {
			Origin    string         `db:"origin"`
			CreatedBy string         `db:"created_by"`
			Data      *datatype.JSON `db:"contract_data"`
		}
		if err := database.GetContext(ctx, &row, `SELECT COALESCE(origin, '') AS origin, created_by, contract_data FROM contracts_effective WHERE did = $1`, did); err != nil {
			return false
		}
		return contractreadpolicy.Scope{Organization: organization, LocalPeer: localPeer}.MayRead(row.Origin, row.CreatedBy, row.Data)
	}
}

// ContractEventOwner returns the owner of a contract for its webhook events
// (DCS-FR-CSA-23): the creating organization and the responsible parties. It
// returns nil when the contract is unknown.
func ContractEventOwner(database *sqlx.DB) func(ctx context.Context, did string) json.RawMessage {
	return func(ctx context.Context, did string) json.RawMessage {
		var owner []byte
		if err := database.GetContext(ctx, &owner, `SELECT json_build_object('created_by', created_by, 'responsible', responsible) FROM contracts_effective WHERE did = $1`, did); err != nil {
			return nil
		}
		return owner
	}
}

func authorizeContractReadDocument(ctx context.Context, database *sqlx.DB, document identity.DIDDocument, did string) error {
	peer, err := document.GetID()
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	return authorizeContractRead(ctx, database, peer, did)
}

// readableContractDIDs is nil only for the existing privileged cross-organization roles.
func readableContractDIDs(ctx context.Context, database *sqlx.DB) (map[string]bool, error) {
	scope, ok := contractreadpolicy.FromContext(ctx)
	if !ok {
		return nil, contractworkflowengine.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	if scope.Privileged() {
		return nil, nil
	}
	var params []any
	query := "SELECT did FROM contracts_effective WHERE " + contractreadpolicy.SQL(ctx, "contracts_effective.did", &params)
	var dids []string
	if err := database.SelectContext(ctx, &dids, query, params...); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	allowed := make(map[string]bool, len(dids))
	for _, did := range dids {
		allowed[did] = true
	}
	return allowed, nil
}

func filterContractAuditChains(chains [][]datatype.AuditLogEntry, allowed map[string]bool) [][]datatype.AuditLogEntry {
	if allowed == nil {
		return chains
	}
	filtered := make([][]datatype.AuditLogEntry, 0, len(chains))
	for _, chain := range chains {
		entries := make([]datatype.AuditLogEntry, 0, len(chain))
		for _, entry := range chain {
			if entry.DID != nil && allowed[*entry.DID] {
				entries = append(entries, entry)
			}
		}
		if len(entries) > 0 {
			filtered = append(filtered, entries)
		}
	}
	return filtered
}
