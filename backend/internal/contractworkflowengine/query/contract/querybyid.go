package contract

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

// ErrContractAccessDenied is returned when the caller is not an authorized
// party of the contract (DCS: created contracts are accessible only to
// authorized parties). Mapped to HTTP 403 by the service layer.
var ErrContractAccessDenied = errors.New("not authorized to access this contract")

// privilegedReadRoles may read any contract regardless of party membership:
// the Sys.* machine roles (cross-org automation), the system administrator,
// and the Auditor (whose function is org-independent audit access).
var privilegedReadRoles = map[userrole.UserRole]bool{
	userrole.SystemContractCreator:  true,
	userrole.SystemContractReviewer: true,
	userrole.SystemContractApprover: true,
	userrole.SystemContractManager:  true,
	userrole.SystemContractSigner:   true,
	userrole.SystemAdministrator:    true,
	userrole.Auditor:                true,
}

type GetByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
	// Internal marks a trusted in-process caller (e.g. the dcs-to-dcs
	// synchronizer reading as "System") that bypasses party read-scoping.
	// Never set from an HTTP request path.
	Internal bool
	// LocalPeer is this instance's own peer DID. Contracts whose Origin is a
	// DIFFERENT peer were adopted via trusted-peer sync — the remote origin
	// only syncs to its responsible peers, so an adopted copy is by
	// construction shared with this instance's organization and is readable
	// by its authenticated users. Party scoping applies strictly to
	// contracts originated locally.
	LocalPeer string
}

type GetByIDResult struct {
	DID             string
	ContractVersion int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContractData    *datatype.JSON
	Negotiations    []db.NegotiationData
	TemplateDID     string
	TemplateVersion int
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Responsible     *db.Responsible
	Origin          string
	// TargetID is the registered target system this contract deploys to
	// (ADR-25); nil until one is designated.
	TargetID *string
}

type GetByIDHandler struct {
	Ctx   context.Context
	DB    *sqlx.DB
	CRepo db.ContractRepo
	NRepo db.NegotiationRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadDataByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract data: %w", err)
	}

	if !callerMayReadContract(query, data) {
		// The denial itself is auditable access history ("the access denial
		// is logged with timestamp"), so commit the denied-event before
		// returning the sentinel.
		deniedEvt := contractevents.RetrieveByIDDeniedEvent{
			DID:         query.DID,
			RetrievedBy: query.RetrievedBy,
			OccurredAt:  time.Now().UTC(),
			HolderDID:   query.HolderDID,
			UserRoles:   query.UserRoles,
		}
		if err := event.Create(h.Ctx, tx, deniedEvt, componenttype.ContractWorkflowEngine); err != nil {
			return nil, fmt.Errorf("could not create denied event: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit denied event: %w", err)
		}
		return nil, ErrContractAccessDenied
	}

	negotiations, err := h.NRepo.ReadAllByContractDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get negotiations: %w", err)
	}

	evt := contractevents.RetrieveByIDEvent{
		DID:         query.DID,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(h.Ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	state, err := contractstate.NewContractState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract state: %w", err)
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if data.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*data.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	result := &GetByIDResult{
		DID:             query.DID,
		ContractVersion: data.ContractVersion,
		State:           state,
		Name:            data.Name,
		Description:     data.Description,
		CreatedBy:       data.CreatedBy,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
		ContractData:    data.ContractData,
		TemplateDID:     data.TemplateDID,
		TemplateVersion: data.TemplateVersion,
		Negotiations:    negotiations,
		StartDate:       data.StartDate,
		ExpDate:         data.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: data.ExpNoticePeriod,
		Responsible:     data.Responsible,
		TargetID:        data.TargetID,
		Origin:          data.Origin,
	}
	return result, nil
}

// callerMayReadContract enforces party read-scoping: the caller's
// organization (middleware.GetParticipantID, the OID4VP-disclosed
// organization claim — the same value persisted as CreatedBy on create) must
// be the contract's creating organization or one of the organizations listed
// in the contract document's top-level "dcs:parties" array. Privileged
// org-independent roles (Sys.* automation, Sys. Administrator, Auditor)
// bypass the check.
func callerMayReadContract(query GetByIDQry, data *db.Contract) bool {
	if query.Internal {
		return true
	}
	return CallerMayReadContract(query.RetrievedBy, query.UserRoles, query.LocalPeer, data)
}

// CallerMayReadContract is the party read-scoping rule shared by direct
// retrieval and the bundle exporter's related/ member filter.
//
// A party has two identity keys and the rule uses both, because neither
// implies the other:
//
//   - WHICH INSTANCE — the did:web. localPeer is this deployment's own peer
//     DID; a contract whose Origin is a different peer was adopted from that
//     peer and is readable here by construction, so the organization check
//     never applies to it. This is the only key that crosses instances.
//   - WHICH ORGANIZATION WITHIN IT — retrievedBy, the caller's OID4VP
//     organization claim, matched against the creating organization and the
//     dcs:legalName of the contract's party nodes. Several organizations
//     share one instance and must not read each other's contracts, which is
//     what the party read-scoping scenarios in features/03_contract_creation
//     exercise: two callers of this same instance, same roles, differing only
//     in that claim.
//
// Collapsing to did:web alone would make every organization on an instance a
// reader of every contract on it. Collapsing to organization alone would
// leave adopted contracts unreadable, since a peer's organizations are not
// this instance's. Privileged org-independent roles bypass the organization
// half only.
func CallerMayReadContract(retrievedBy string, userRoles userrole.UserRoles, localPeer string, data *db.Contract) bool {
	if localPeer != "" && data.Origin != "" && data.Origin != localPeer {
		return true
	}
	for _, role := range userRoles {
		if privilegedReadRoles[role] {
			return true
		}
	}
	if retrievedBy != "" && retrievedBy == data.CreatedBy {
		return true
	}
	for _, party := range contractParties(data.ContractData) {
		if retrievedBy != "" && retrievedBy == party {
			return true
		}
	}
	return false
}

// contractParties reads the organizations named by the typed dcs:CompanyParty
// nodes under the contract document's top-level "dcs:parties". Absence simply
// means no additional parties beyond the creating organization.
//
// A node contributes only if it carries dcs:legalName. Nodes keyed by did:web
// carry it too since bindOriginatorParty stamps both keys, so the originator
// is visible here; a node with a role but no name is an attribution node for
// a party this instance cannot resolve to a local organization, and it grants
// nobody read access.
func contractParties(raw *datatype.JSON) []string {
	if raw == nil {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil
	}
	entries, ok := doc["dcs:parties"].([]any)
	if !ok {
		return nil
	}
	parties := make([]string, 0, len(entries))
	for _, entry := range entries {
		node, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := node["dcs:legalName"].(string); ok && name != "" {
			parties = append(parties, name)
		}
	}
	return parties
}
