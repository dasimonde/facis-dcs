package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"
)

type SubmitCmd struct {
	DID          string                           `json:"did"`
	UpdatedAt    time.Time                        `json:"updated_at"`
	SubmittedBy  string                           `json:"submitted_by"`
	ActionFlag   *actionflag.ActionFlag           `json:"action_flag"`
	Comments     []string                         `json:"comments"`
	Annotations  []contractevents.BlockAnnotation `json:"annotations,omitempty"`
	ContractData *datatype.JSON                   `json:"contract_data"`
	HolderDID    string                           `json:"holder_did"`
	UserRoles    userrole.UserRoles               `json:"user_roles"`
	CauserDID    string                           `json:"causer_did"`
}

type Submitter struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	NORepo      db.NegotiationOutboundRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Submitter) Handle(ctx context.Context, cmd SubmitCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere)
		}
		return fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
	}

	hasSubmittedContractData := cmd.ContractData != nil && cmd.ContractData.IsNotNullValue()
	if hasSubmittedContractData && !canSubmitUpdatedContractData(processData.State) {
		return errors.New("contract data can only be submitted in draft or rejected state")
	}
	if hasSubmittedContractData {
		if err := requireUnsettledAgreement(ctx, tx, h.CRepo, h.SRepo, localPeer, cmd.DID); err != nil {
			return err
		}
	}

	// The transition table (contractstate.Transitions) is the single source
	// of truth for which states Submit may be called from at all. It does
	// NOT replace the imperative branching below — that logic stays,
	// unchanged, to decide (given the current state and payload) exactly
	// which of the table's allowed outcomes applies; see command package doc.
	currentState := contractstate.ContractState(processData.State)
	if err := contractstate.ValidateTransition(currentState, contractstate.EventSubmit); err != nil {
		return err
	}

	// Submit is intentionally overloaded: its effect depends entirely on the
	// contract's current state (state pattern via if/else, not polymorphism).
	// See docs/backend architecture doc, section "Contract Workflow Engine".
	var nextState contractstate.ContractState
	// Draft and Offered submit identically: both start the negotiation round
	// (see transition.go's Draft -> Negotiation and Offered -> Negotiation
	// edges). Submitting an already-offered contract simply mirrors submitting
	// a draft.
	if processData.State == contractstate.Draft.String() || processData.State == contractstate.Offered.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.SystemContractCreator) {
			return errors.New("invalid user permission")
		}

		// This avoids that state changes on different DCS are possible
		if cmd.CauserDID == localPeer && cmd.SubmittedBy != processData.CreatedBy {
			return ErrNotCreator
		}

		if err := requireAssemblyMetadata(ctx, tx, h.CRepo, cmd.DID); err != nil {
			return err
		}
		contractData, err := h.contractDataForSemanticValidation(ctx, tx, cmd)
		if err != nil {
			return err
		}
		if err := validation.ValidateContractSemantics(contractData); err != nil {
			return fmt.Errorf("contract semantic validation failed: %w", err)
		}
		if err := validation.RequireHubConformance(ctx, contractData); err != nil {
			return fmt.Errorf("contract submission blocked: %w", err)
		}

		// Submission into NEGOTIATION finalizes the participating parties, so seed
		// the signable structure for them — one party node per party (origin and
		// counterparty) and the AcroForm signature field naming it
		// (dcs:signatoryName == the party's DCS instance DID) — which prepare
		// renders and the signatory's wallet signs (ADR-13).
		//
		// A submitted draft (persisted just above) is the document the client
		// assembled, which is where the party nodes go missing, so this reads back
		// what was stored and puts them back.
		if err := reseedPartiesAndSignatureFields(ctx, tx, h.CRepo, cmd.DID); err != nil {
			return err
		}

		nextState = contractstate.Negotiation

	} else if processData.State == contractstate.Rejected.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.SystemContractCreator) {
			return errors.New("invalid user permission")
		}

		// This avoids that state changes on different DCS are possible
		if cmd.CauserDID == localPeer && cmd.SubmittedBy != processData.CreatedBy {
			return ErrNotCreator
		}

		if err := requireAssemblyMetadata(ctx, tx, h.CRepo, cmd.DID); err != nil {
			return err
		}
		contractData, err := h.contractDataForSemanticValidation(ctx, tx, cmd)
		if err != nil {
			return err
		}
		if err := validation.ValidateContractSemantics(contractData); err != nil {
			return fmt.Errorf("contract semantic validation failed: %w", err)
		}
		if err := validation.RequireHubConformance(ctx, contractData); err != nil {
			return fmt.Errorf("contract submission blocked: %w", err)
		}

		err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen review tasks")
		}

		err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID, processData.ContractVersion)
		if err != nil {
			return errors.New("could not reopen negotiation tasks")
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen approval tasks")
		}

		nextState = contractstate.Negotiation

	} else if processData.State == contractstate.Negotiation.String() {

		// The counterparty drives its inbound contract as Contract Manager, so
		// the Manager closes the round it negotiates (DCS-IR-CWE-03).
		if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.SystemContractCreator, userrole.ContractNegotiator, userrole.ContractReviewer, userrole.SystemContractReviewer, userrole.ContractManager) {
			return errors.New("invalid user permission")
		}

		// The settlement gate. A task exists for this round only because this
		// instance engaged with it — authored the contract, accepted the inbound
		// offer, or proposed a redline on it — so its absence is the honest
		// "nobody has entered this round yet", not a permission fault.
		isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID, processData.ContractVersion)
		if err != nil {
			return fmt.Errorf("could not validate negotiator: %w", err)
		}

		if !isValidNegotiator {
			return ErrNegotiationNotSettled
		}

		hasOpenNegotiations, err := h.NRepo.HasOpenNegotiationDecisions(ctx, tx, cmd.DID, processData.ContractVersion, cmd.CauserDID, cmd.SubmittedBy)
		if err != nil {
			return fmt.Errorf("could not check open negotiations: %w", err)
		}

		if hasOpenNegotiations {
			return errors.New("not all negotiations are processed")
		}

		// Submit gate: every inter-enterprise Proposal/Decision for
		// this contract must already have peer ACK before the round closes.
		err = requireConfirmedNegotiationDelivery(ctx, tx, h.NORepo, localPeer, cmd.DID)
		if err != nil {
			return err
		}

		err = h.NTRepo.UpdateState(ctx, tx, processData.DID, cmd.CauserDID, processData.ContractVersion, negotiationtaskstate.Accepted.String())
		if err != nil {
			return fmt.Errorf("could not update negotiation task: %w", err)
		}

		existOpenTasks, err := h.NTRepo.AnyTasksInState(ctx, tx, processData.DID, processData.ContractVersion, negotiationtaskstate.Open.String())
		if err != nil {
			return fmt.Errorf("could not check if review task exists: %w", err)
		}

		if !existOpenTasks {

			hasNegotiations, err := h.NRepo.HasNegotiationForContractVersion(ctx, tx, cmd.DID, processData.ContractVersion)
			if err != nil {
				return fmt.Errorf("could not check if negotiation exists: %w", err)
			}

			if hasNegotiations {
				// All negotiators have responded and there are accepted change
				// requests to fold in: snapshot the current row to contract_history,
				// merge the changes, and bump contract_version. The contract stays in
				// NEGOTIATION (nextState is left unset) rather than advancing to
				// SUBMITTED, since the merged result itself starts a new round.
				err = h.CRepo.CreateHistoryEntryForDID(ctx, tx, processData.DID, cmd.SubmittedBy)
				if err != nil {
					return fmt.Errorf("could not create history entry for did %s: %w", cmd.DID, err)
				}

				updatedData, superseded, err := negotiationmerging.MergeChangeRequests(ctx, tx, h.CRepo, h.NRepo, cmd.DID, processData.ContractVersion)
				if err != nil {
					return fmt.Errorf("could not merge change requests: %w", err)
				}

				// Only a merge that rewrites the document is refused on a settled
				// agreement. Redlines accepted before this instance committed are
				// the ones that can still get here (negotiate refuses new ones),
				// and folding them in now would move the document off the artifact
				// a party signed, or off the version this instance agreed to and
				// shipped a settlement for.
				if updatedData.ContractData != nil {
					if err := requireUnsettledAgreement(ctx, tx, h.CRepo, h.SRepo, localPeer, cmd.DID); err != nil {
						return err
					}
				}

				updatedData.ContractVersion = processData.ContractVersion + 1
				err = h.CRepo.Update(ctx, tx, *updatedData)
				if err != nil {
					return fmt.Errorf("could not update contract version: %w", err)
				}

				// The merged result starts a new round, and the tasks key on the
				// round: a party that engaged with this one still owes a response to
				// the next, so its task travels to the new version and reopens.
				if err := h.NTRepo.RollForward(ctx, tx, processData.DID, processData.ContractVersion, updatedData.ContractVersion); err != nil {
					return fmt.Errorf("could not carry negotiation tasks to the new contract version: %w", err)
				}

				// The merge does not fold a redline into the stored document, it
				// replaces the document with the redline (mergeContractDataChange
				// returns the change), so the merged result is a client-assembled
				// document like any other and needs its party nodes back.
				if updatedData.ContractData != nil {
					if err := reseedPartiesAndSignatureFields(ctx, tx, h.CRepo, cmd.DID); err != nil {
						return err
					}
				}

				// Shared Contract Details live on the contracts row and must
				// also sit in dcs:metadata before the post-merge PDF ships, so
				// the peer's reconstruct-and-verify and the settlement digest
				// bind the same shared fields both enterprises signed.
				if err := stampMergedSharedDetails(ctx, tx, h.CRepo, cmd.DID); err != nil {
					return err
				}

				// Last-accepted-wins leaves accepted requests whose content the
				// merged version does not carry. Their decision rows still read
				// ACCEPTED, so the discard is annotated on the losing request and
				// put on the audit trail; without it the record asserts a party
				// agreed to wording the contract never contained.
				if len(superseded) > 0 {
					if err := h.NRepo.MarkSuperseded(ctx, tx, superseded); err != nil {
						return fmt.Errorf("could not record superseded change requests: %w", err)
					}

					supersededEvt := contractevents.NegotiationChangeSupersededEvent{
						DID:             cmd.DID,
						HolderDID:       cmd.HolderDID,
						ContractVersion: processData.ContractVersion,
						MergedVersion:   processData.ContractVersion + 1,
						Superseded:      superseded,
						SubmittedBy:     cmd.SubmittedBy,
						OccurredAt:      time.Now().UTC(),
						UserRoles:       cmd.UserRoles,
					}
					if err := event.Create(ctx, tx, supersededEvt, componenttype.ContractWorkflowEngine); err != nil {
						return fmt.Errorf("could not create superseded change request event: %w", err)
					}
				}

				evt := contractevents.IncreaseContractVersionEvent{
					DID:                cmd.DID,
					OldContractVersion: processData.ContractVersion,
					NewContractVersion: processData.ContractVersion + 1,
					SubmittedBy:        cmd.SubmittedBy,
					OccurredAt:         time.Now().UTC(),
					HolderDID:          cmd.HolderDID,
					UserRoles:          cmd.UserRoles,
				}
				err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
				if err != nil {
					return fmt.Errorf("could not create event: %w", err)
				}

			} else {
				// Every task minted for this round is accepted and there is nothing
				// left to merge: the round is settled. "No open tasks" is evidence
				// here because a task is minted only by an act of engagement, so an
				// empty round was already refused above.
				nextState = contractstate.Submitted
			}
		}

	} else if processData.State == contractstate.Submitted.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractReviewer, userrole.SystemContractReviewer) {
			return errors.New("invalid user permission")
		}

		isValid, err := h.RTRepo.IsValidReviewer(ctx, tx, processData.DID, cmd.CauserDID)
		if err != nil {
			return err
		}

		if !isValid {
			return errors.New("invalid user")
		}

		if cmd.ActionFlag != nil {
			switch *cmd.ActionFlag {
			case actionflag.Approval:
				err = h.RTRepo.UpdateState(ctx, tx, processData.DID, cmd.CauserDID, contractstate.Approved.String())
				if err != nil {
					return fmt.Errorf("could not update approval task: %w", err)
				}
				existOpenTasks, err := h.RTRepo.AnyTasksInState(ctx, tx, processData.DID, reviewtaskstate.Open.String())
				if err != nil {
					return fmt.Errorf("could not check if review task exists: %w", err)
				}
				if !existOpenTasks {
					nextState = contractstate.Reviewed
				}
			case actionflag.Reject:
				err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}
				err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID, processData.ContractVersion)
				if err != nil {
					return err
				}
				err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}
				// The round this reopens is the one whose close settled the
				// agreement, so reopening it takes that agreement back: without
				// this the reviewer's rejection lands in NEGOTIATION where every
				// document edit is refused for a version this instance still
				// stands behind.
				if err := withdrawOwnSettlement(ctx, tx, h.SRepo, localPeer, cmd.DID); err != nil {
					return err
				}
				nextState = contractstate.Negotiation
			}

		} else {
			return errors.New("action flags is missing")
		}

	} else if processData.State == contractstate.Reviewed.String() {

		if !cmd.UserRoles.HasRoles(userrole.ContractApprover, userrole.SystemContractApprover) {
			return errors.New("invalid user permission")
		}

		isValid, err := h.ATRepo.IsValidApprover(ctx, tx, processData.DID, cmd.CauserDID)
		if err != nil {
			return err
		}

		if !isValid {
			return errors.New("invalid user")
		}

		err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		nextState = contractstate.Submitted

	} else {
		// Unreachable: the ValidateTransition guard above already rejects any
		// state without a declared EventSubmit outcome. Kept as a
		// defense-in-depth fallback.
		return fmt.Errorf("%w: submit is not allowed from state %s", contractstate.ErrInvalidTransition, processData.State)
	}

	if len(nextState) > 0 && processData.State != nextState.String() {
		if err := contractstate.ValidateOutcome(currentState, contractstate.EventSubmit, nextState); err != nil {
			return err
		}
		err = h.CRepo.UpdateState(ctx, tx, cmd.DID, processData.State, nextState.String())
		if err != nil {
			return fmt.Errorf("could not update contract state: %w", err)
		}
	}

	annotations, err := normalizeSubmitAnnotations(ctx, tx, h, cmd, processData)
	if err != nil {
		return err
	}

	evt := contractevents.SubmitEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		SubmittedBy:     cmd.SubmittedBy,
		PreviousState:   processData.State,
		NewState:        nextState.String(),
		ActionFlag:      cmd.ActionFlag,
		Comments:        cmd.Comments,
		Annotations:     annotations,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func (h *Submitter) contractDataForSemanticValidation(ctx context.Context, tx *sqlx.Tx, cmd SubmitCmd) (*datatype.JSON, error) {
	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		normalizedContractData, err := validation.NormalizeContractDataForPersistence(cmd.ContractData, cmd.DID, false)
		if err != nil {
			return nil, fmt.Errorf("contract data validation failed: %w", err)
		}
		stored, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
		if err != nil {
			return nil, fmt.Errorf("could not read contract data: %w", err)
		}
		// The submitted draft replaces the document wholesale, so the bundle it
		// was created against travels from the stored copy (CarrySemanticBundle).
		normalizedContractData, err = validation.CarrySemanticBundle(stored.ContractData, normalizedContractData)
		if err != nil {
			return nil, fmt.Errorf("could not carry the pinned Semantic Hub bundle forward: %w", err)
		}
		// The submitted document is a save like an update: the same parent
		// checks, and a history entry of the replaced version with the actor
		// (DCS-FR-CWE-08, DCS-FR-CWE-02).
		if err := checkContractParent(ctx, tx, h.CRepo, cmd.DID, normalizedContractData); err != nil {
			return nil, err
		}
		if err := h.CRepo.CreateHistoryEntryForDID(ctx, tx, cmd.DID, cmd.SubmittedBy); err != nil {
			return nil, fmt.Errorf("could not create history entry: %w", err)
		}
		updateData := db.ContractUpdateData{
			DID:          cmd.DID,
			ContractData: normalizedContractData,
		}
		if err := h.CRepo.Update(ctx, tx, updateData); err != nil {
			return nil, fmt.Errorf("could not update submitted contract data: %w", err)
		}
		return normalizedContractData, nil
	}

	contractData, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract data: %w", err)
	}
	return contractData.ContractData, nil
}

// ErrMissingRequiredMetadata is returned when an assembled contract is
// submitted without the metadata every contract carries, or with a validity
// period that does not hold.
var ErrMissingRequiredMetadata = errors.New("contract metadata is incomplete")

// requireAssemblyMetadata checks the required metadata of an assembled
// contract before it leaves draft (DCS-FR-CWE-03): a name, a description (the
// two fields the contract form requires), a validity period that ends after it
// starts, and a notice period that is not negative.
func requireAssemblyMetadata(ctx context.Context, tx *sqlx.Tx, repo db.ContractRepo, did string) error {
	contract, err := repo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read contract metadata: %w", err)
	}
	var problems []string
	for field, value := range map[string]*string{"name": contract.Name, "description": contract.Description} {
		if value == nil || strings.TrimSpace(*value) == "" {
			problems = append(problems, field+" is missing")
		}
	}
	if contract.StartDate != nil && contract.ExpDate != nil && !contract.ExpDate.After(*contract.StartDate) {
		problems = append(problems, "expiration date is not after the start date")
	}
	if contract.ExpNoticePeriod != nil && *contract.ExpNoticePeriod < 0 {
		problems = append(problems, "notice period is negative")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%w: %s", ErrMissingRequiredMetadata, strings.Join(problems, "; "))
	}
	return nil
}

func canSubmitUpdatedContractData(state string) bool {
	return state == contractstate.Draft.String() || state == contractstate.Rejected.String()
}

// normalizeSubmitAnnotations trims annotations and, when any are present,
// checks each block_id exists in the contract document for this version.
func normalizeSubmitAnnotations(
	ctx context.Context,
	tx *sqlx.Tx,
	h *Submitter,
	cmd SubmitCmd,
	processData *db.ContractProcessData,
) ([]contractevents.BlockAnnotation, error) {
	if len(cmd.Annotations) == 0 {
		return nil, nil
	}

	out := make([]contractevents.BlockAnnotation, 0, len(cmd.Annotations))
	for _, a := range cmd.Annotations {
		blockID := strings.TrimSpace(a.BlockID)
		comment := strings.TrimSpace(a.Comment)
		if blockID == "" || comment == "" {
			return nil, errors.New("each annotation requires a non-empty block_id and comment")
		}
		out = append(out, contractevents.BlockAnnotation{BlockID: blockID, Comment: comment})
	}

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract data for annotation validation: %w", err)
	}

	var raw any
	if cmd.ContractData != nil && cmd.ContractData.IsNotNullValue() {
		raw = cmd.ContractData
	} else if contract.ContractData != nil {
		raw = contract.ContractData
	}

	known, err := collectDocumentBlockIDs(raw)
	if err != nil {
		return nil, fmt.Errorf("could not index contract blocks for annotations: %w", err)
	}

	for _, a := range out {
		if !known[a.BlockID] {
			return nil, fmt.Errorf("annotation block_id %q is not present in contract version %d", a.BlockID, processData.ContractVersion)
		}
	}

	return out, nil
}

func collectDocumentBlockIDs(contractDocument any) (map[string]bool, error) {
	doc, err := normalizeSubmitDocument(contractDocument)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	var walk func(any)

	walk = func(current any) {
		switch value := current.(type) {
		case map[string]any:
			if id, ok := value["@id"].(string); ok {
				id = strings.TrimSpace(id)
				if id != "" {
					out[id] = true
				}
			}
			for _, nested := range value {
				walk(nested)
			}
		case []any:
			for _, nested := range value {
				walk(nested)
			}
		}
	}
	walk(doc)

	return out, nil
}

func normalizeSubmitDocument(raw any) (map[string]any, error) {
	switch v := raw.(type) {
	case nil:
		return nil, errors.New("contract document is required")
	case map[string]any:
		return v, nil
	case *datatype.JSON:
		if v == nil {
			return nil, errors.New("contract document is required")
		}

		var doc map[string]any
		if err := json.Unmarshal(*v, &doc); err != nil {
			return nil, err
		}

		return doc, nil
	case datatype.JSON:
		var doc map[string]any
		if err := json.Unmarshal(v, &doc); err != nil {
			return nil, err
		}

		return doc, nil
	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("unsupported contract document type %T: %w", raw, err)
		}

		var doc map[string]any
		if err := json.Unmarshal(bytes, &doc); err != nil {
			return nil, err
		}

		return doc, nil
	}
}
