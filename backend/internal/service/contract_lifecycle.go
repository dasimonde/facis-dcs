package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	trustgate "digital-contracting-service/internal/dcstodcs"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"
)

// lifecycleActorField maps each lifecycle event type to the body field that names its actor.
// Read, search, audit and evidence events are absent on purpose: the lifecycle read
// is open to every contract party and must not disclose who looked at the contract.
var lifecycleActorField = map[string]string{
	cweeventtype.Create.String():           "created_by",
	cweeventtype.Offer.String():            "offered_by",
	cweeventtype.Withdraw.String():         "withdrawn_by",
	cweeventtype.Submit.String():           "submitted_by",
	cweeventtype.Negotiation.String():      "negotiated_by",
	cweeventtype.AcceptOffer.String():      "accepted_by",
	cweeventtype.AcceptRespond.String():    "accepted_by",
	cweeventtype.RejectRespond.String():    "rejected_by",
	cweeventtype.WithdrawRespond.String():  "withdrawn_by",
	cweeventtype.Approve.String():          "approved_by",
	cweeventtype.Reject.String():           "rejected_by",
	cweeventtype.Terminate.String():        "terminated_by",
	cweeventtype.Renew.String():            "renewed_by",
	cweeventtype.ContractExpired.String():  "",
	cweeventtype.Activate.String():         "activated_by",
	cweeventtype.StoreArchived.String():    "stored_by",
	cweeventtype.PublishCatalogue.String(): "published_by",
	smeventtype.Applied.String():           "applied_by",
	smeventtype.Revoke.String():            "revoked_by",
	// A counterparty instance refused a termination notice for good, so its
	// copy is not terminated (DCS-NFR-BR-06).
	trustgate.PeerExchangeRefusedEvent{}.EventType(): "peer_did",
}

var lifecycleComponents = []componenttype.ComponentType{
	componenttype.ContractWorkflowEngine,
	componenttype.SignatureManagement,
	componenttype.ContractStorageArchive,
}

func (s *contractWorkflowEnginesrvc) Lifecycle(ctx context.Context, req *cwe.LifecyclePayload) ([]*cwe.ContractLifecycleEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, req.Did); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, cwe.MakeInternalError(err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var entries []datatype.AuditLogEntry
	for _, component := range lifecycleComponents {
		chain, err := s.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, component, req.Did)
		if err != nil {
			return nil, cwe.MakeInternalError(err)
		}
		entries = append(entries, chain...)
	}
	return lifecycleEvents(entries), nil
}

// lifecycleEvents keeps only lifecycle transitions, newest first, and copies no body field
// other than the actor, the occurrence time and the state change.
func lifecycleEvents(entries []datatype.AuditLogEntry) []*cwe.ContractLifecycleEvent {
	type dated struct {
		at  time.Time
		evt *cwe.ContractLifecycleEvent
	}
	items := make([]dated, 0, len(entries))
	for _, entry := range entries {
		actorField, ok := lifecycleActorField[entry.EventType]
		if !ok {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(entry.EventData, &body)
		at := entry.CreatedAt
		if raw := bodyString(body, "occurred_at"); raw != nil {
			if parsed, err := time.Parse(time.RFC3339Nano, *raw); err == nil {
				at = parsed
			}
		}
		evt := &cwe.ContractLifecycleEvent{
			EventType:     entry.EventType,
			OccurredAt:    at.UTC().Format(time.RFC3339),
			PreviousState: bodyString(body, "previous_state"),
			NewState:      bodyString(body, "new_state"),
		}
		if actorField != "" {
			evt.Actor = bodyString(body, actorField)
		}
		items = append(items, dated{at: at, evt: evt})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	events := make([]*cwe.ContractLifecycleEvent, len(items))
	for i, item := range items {
		events[i] = item.evt
	}
	return events
}

func bodyString(body map[string]any, key string) *string {
	if value, ok := body[key].(string); ok && value != "" {
		return &value
	}
	return nil
}
