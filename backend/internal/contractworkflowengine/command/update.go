package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

// ErrContractHierarchyCycle is returned when an update would make a contract's
// locally resolvable parent chain point back at the contract itself. It is
// mapped to a 4xx client error by the HTTP layer.
var ErrContractHierarchyCycle = errors.New("contract parent chain contains a cycle")

type UpdateCmd struct {
	DID             string                             `json:"did"`
	UpdatedAt       time.Time                          `json:"updated_at"`
	UpdatedBy       string                             `json:"updated_by"`
	StartDate       *time.Time                         `json:"start_date"`
	ExpDate         *time.Time                         `json:"exp_date"`
	ExpPolicy       *expirationpolicy.ExpirationPolicy `json:"exp_policy"`
	ExpNoticePeriod *int                               `json:"exp_notice_period"`
	Name            *string                            `json:"name"`
	Description     *string                            `json:"description"`
	ContractData    *datatype.JSON                     `json:"contract_data"`
	HolderDID       string                             `json:"holder_did"`
	UserRoles       userrole.UserRoles                 `json:"user_roles"`
	CauserDID       string                             `json:"causer_did"`
}

type Updater struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {

	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		normalizedContractData, err := validation.NormalizeContractDataForPersistence(cmd.ContractData, cmd.DID, true)
		if err != nil {
			return fmt.Errorf("contract data validation failed: %w", err)
		}
		cmd.ContractData = normalizedContractData
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	oldData, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	if oldData.Origin != localPeer && cmd.CauserDID != oldData.Origin {
		/*
			Unlike every other state-mutating handler in this package, Update does
			NOT forward to the Origin peer — it is simply rejected on non-Origin
			nodes. Updates must be performed directly on the contract's owner peer.
		*/

		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		return fmt.Errorf("updates are just allowed contract's owner peer")
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(oldData.State), contractstate.EventUpdate); err != nil {
		return err
	}

	// Reject an update whose (locally resolvable) parent chain would loop
	// back to this contract. Non-local parents are simply not walked further —
	// cross-instance parents are legitimate and unresolvable here by design.
	if parentDID := extractParentContractDID(cmd.ContractData); parentDID != "" {
		if err := h.checkNoParentCycle(ctx, tx, cmd.DID, parentDID); err != nil {
			return err
		}
	}

	if cmd.ExpDate != nil {
		tomorrow := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if cmd.ExpDate.Before(tomorrow) {
			return fmt.Errorf("expiration date must be at least one day in the future")
		}
	}

	if cmd.StartDate != nil {
		tomorrow := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		if cmd.StartDate.Before(tomorrow) {
			return fmt.Errorf("start date must be at least one day in the future")
		}
	}

	if cmd.StartDate != nil && cmd.ExpDate != nil {
		if !cmd.ExpDate.After(*cmd.StartDate) {
			return fmt.Errorf("expiration date must be after start date")
		}
	}

	var oldExpPolicy *expirationpolicy.ExpirationPolicy
	if oldData.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*oldData.ExpPolicy)
		if err != nil {
			return fmt.Errorf("could not parse expiration policy: %w", err)
		}
		oldExpPolicy = &policy
	}

	var expPolicy *string
	if cmd.ExpPolicy != nil {
		s := cmd.ExpPolicy.String()
		expPolicy = &s
	}

	// A save replaces the document with the one the client assembled from its
	// editor state, which carries no party nodes, no signature fields and no
	// Semantic Hub pin — the client neither authors nor models them. Put them
	// back before the row is written, so a document that a save stripped is
	// never stored without the nodes a later signature is attributed to
	// (SeedPartiesAndSignatureFields) or without the bundle it was created
	// against (CarrySemanticBundle).
	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		cmd.ContractData, err = validation.CarrySemanticBundle(oldData.ContractData, cmd.ContractData)
		if err != nil {
			return fmt.Errorf("could not carry the pinned Semantic Hub bundle forward: %w", err)
		}
		if oldData.Responsible != nil {
			seeded, changed, err := SeedPartiesAndSignatureFields(*cmd.ContractData, oldData.Responsible.GetParties())
			if err != nil {
				return fmt.Errorf("could not seed contract parties and signature fields: %w", err)
			}
			if changed {
				cmd.ContractData = &seeded
			}
		}
	}

	newData := db.ContractUpdateData{
		DID:             cmd.DID,
		Name:            cmd.Name,
		Description:     cmd.Description,
		StartDate:       cmd.StartDate,
		ExpDate:         cmd.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: cmd.ExpNoticePeriod,
		ContractData:    cmd.ContractData,
	}
	err = h.CRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update contract data: %w", err)
	}

	evt := contractevents.UpdateEvent{
		DID:                cmd.DID,
		OldName:            oldData.Name,
		NewName:            cmd.Name,
		OldDescription:     oldData.Description,
		NewDescription:     cmd.Description,
		OldContractData:    oldData.ContractData,
		NewContractData:    cmd.ContractData,
		OldStartDate:       oldData.StartDate,
		NewStartDate:       cmd.StartDate,
		OldExpDate:         oldData.ExpDate,
		NewExpDate:         cmd.ExpDate,
		OldExpPolicy:       oldExpPolicy,
		NewExpPolicy:       cmd.ExpPolicy,
		OldExpNoticePeriod: oldData.ExpNoticePeriod,
		NewExpNoticePeriod: cmd.ExpNoticePeriod,
		UpdatedBy:          cmd.UpdatedBy,
		OccurredAt:         time.Now().UTC(),
		HolderDID:          cmd.HolderDID,
		UserRoles:          cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// checkNoParentCycle walks the parent chain starting at proposedParentDID and
// rejects if it reaches selfDID. Parents that do not resolve locally end the
// walk (cross-instance parents are legitimate and unresolvable here). A visited
// set guards against any pre-existing loop in stored data.
func (h *Updater) checkNoParentCycle(ctx context.Context, tx *sqlx.Tx, selfDID, proposedParentDID string) error {
	visited := map[string]bool{}
	current := proposedParentDID
	for current != "" {
		if current == selfDID {
			return fmt.Errorf("%w: updating %s to reference %s would loop the parent chain back to itself",
				ErrContractHierarchyCycle, selfDID, proposedParentDID)
		}
		if visited[current] {
			return nil
		}
		visited[current] = true

		parent, err := h.CRepo.ReadDataByDID(ctx, tx, current)
		if err != nil {
			// Parent not resolvable locally (e.g. a cross-instance frame): stop.
			return nil
		}
		current = extractParentContractDID(parent.ContractData)
	}
	return nil
}

// extractParentContractDID returns the single dcs:parentContract @id from a
// contract document, or "" when none is present. Accepts both the object form
// ({"@id": "..."}) and a one-element array form.
func extractParentContractDID(data *datatype.JSON) string {
	if data == nil || !data.IsNotNullValue() {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(*data, &doc); err != nil {
		return ""
	}
	switch typed := doc["dcs:parentContract"].(type) {
	case map[string]any:
		id, _ := typed["@id"].(string)
		return base.ResourceKey(id)
	case []any:
		if len(typed) == 0 {
			return ""
		}
		if first, ok := typed[0].(map[string]any); ok {
			id, _ := first["@id"].(string)
			return base.ResourceKey(id)
		}
	}
	return ""
}
