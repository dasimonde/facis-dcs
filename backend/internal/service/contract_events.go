package service

import (
	"context"
	"errors"
	"time"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
)

// contractEventsHeartbeat keeps the stream open through idle proxy timeouts
// and lets the client detect a dead connection.
const contractEventsHeartbeat = 30 * time.Second

// contractEventsMaxAge ends a stream after the access token lifetime, so the
// client reconnects with a current token and a revoked session stops
// receiving changes.
const contractEventsMaxAge = 5 * time.Minute

// Events streams the contract changes the caller may read (DCS-FR-CWE-24,
// DCS-FR-CSA-23). The first event, CONNECTED, confirms the stream; HEARTBEAT
// follows every 30 seconds. Both name no contract. RESYNC, also without a
// contract, ends a stream that lost changes. The read policy is applied
// per change, so a client receives only changes to contracts it may read. The
// stream ends after contractEventsMaxAge and the client reconnects.
func (s *contractWorkflowEnginesrvc) Events(ctx context.Context, _ *contractworkflowengine.ContractEventsRequest, stream contractworkflowengine.EventsServerStream) error {
	if s.Changes == nil {
		return contractworkflowengine.MakeInternalError(errors.New("live contract updates are not configured"))
	}
	peer, err := s.DIDDocument.GetID()
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, contractEventsMaxAge)
	defer cancel()
	changes, stop := s.Changes.Subscribe()
	defer stop()
	heartbeat := time.NewTicker(contractEventsHeartbeat)
	defer heartbeat.Stop()

	if err := stream.SendWithContext(ctx, &contractworkflowengine.ContractChangeEvent{EventType: "CONNECTED"}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			if err := stream.SendWithContext(ctx, &contractworkflowengine.ContractChangeEvent{EventType: "HEARTBEAT"}); err != nil {
				return err
			}
		case change, open := <-changes:
			if !open {
				// This stream lost changes. RESYNC tells the client to reload
				// what it shows; the client then connects again.
				return stream.SendWithContext(ctx, &contractworkflowengine.ContractChangeEvent{EventType: "RESYNC"})
			}
			if authorizeContractRead(ctx, s.DB, peer, change.DID) != nil {
				continue
			}
			if err := stream.SendWithContext(ctx, &contractworkflowengine.ContractChangeEvent{Did: change.DID, EventType: change.EventType}); err != nil {
				return err
			}
		}
	}
}
