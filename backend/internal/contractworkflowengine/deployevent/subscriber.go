// Package deployevent auto-deploys a contract once its signing workflow
// completes (DCS-FR-CWE-06): it subscribes to the signingmanagement
// APPLIED_SIGNATURE event on the NATS event bus and calls the same
// command.Deployer the manual POST /contract/deploy endpoint uses, so both
// paths share one deployment implementation.
package deployevent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"
)

// Subscriber listens for signature-applied events and dispatches an
// automatic deployment for the signed contract.
type Subscriber struct {
	Deployer ContractDeployer
	Gate     *workflowgate.Coordinator
	// LocalPeer is this instance's own did:web, passed on so the multi-signer
	// gate reads the same on this path as on the manual endpoint: without it
	// every declared field looks local, and a federated contract's counterparty
	// slot is demanded from a database that will never hold it.
	LocalPeer string
}

// ContractDeployer is the deployment implementation this subscriber shares with
// the manual POST /contract/deploy endpoint (command.Deployer).
type ContractDeployer interface {
	Deployable(ctx context.Context, did, localPeer string) error
	Handle(ctx context.Context, cmd command.DeployCmd) (*command.DeployResult, error)
}

// Start registers the event handler with the durable sub-client and consumes
// events until the sub-client is closed.
func (s *Subscriber) Start(subClient *event.DurableSubClient) error {
	return subClient.Subscribe(s.consume)
}

// consume handles one event. It returns an error, and the event is delivered
// again, only for a failure that a later attempt can overcome.
func (s *Subscriber) consume(evt cloudevent.Event) error {
	if evt.Type() != smeventtype.Applied.String() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := s.handle(ctx, evt)
	var blocked *workflowgate.BlockedError
	switch {
	case err == nil,
		// A contract that designates no target system is not an error: not every
		// party deploys what it signs — a negotiating peer may simply hold the
		// agreement. Deployment is opt-in, so this is an ordinary outcome
		// (ADR-25) and must not be reported as a failure.
		errors.Is(err, command.ErrNoTargetDesignated),
		// A partially signed contract fires this event after each signature but
		// the last; that is the expected path, not a failure.
		errors.Is(err, command.ErrSigningIncomplete),
		// A redelivered event whose deployment is dispatched already.
		errors.Is(err, command.ErrAlreadyDispatched):
		return nil
	case errors.As(err, &blocked),
		errors.Is(err, contractstate.ErrInvalidTransition),
		errors.Is(err, command.ErrTargetNotRegistered),
		errors.Is(err, command.ErrTargetDisabled):
		// The gate run and the registry decide; a redelivery reads the same.
		log.Printf("contractworkflowengine/deployevent: could not auto-deploy: %v", err)
		return nil
	default:
		return fmt.Errorf("contractworkflowengine/deployevent: could not auto-deploy: %w", err)
	}
}

func (s *Subscriber) handle(ctx context.Context, evt cloudevent.Event) error {
	// The outbox publisher republishes the persisted json.RawMessage event
	// payload as the CloudEvent's data verbatim (base/event/
	// cloudeventprovider.go: SetData(ApplicationJSON, data) with a
	// json.RawMessage does not go through the []byte/base64 branch, since
	// json.RawMessage is a distinct named type from []byte), so DataAs
	// decodes straight into the target struct.
	var envelope struct {
		DID string `json:"did"`
	}
	if err := evt.DataAs(&envelope); err != nil {
		return err
	}
	if envelope.DID == "" {
		return nil
	}
	// The gate is dispatched only for a contract that may deploy now.
	if err := s.Deployer.Deployable(ctx, envelope.DID, s.LocalPeer); err != nil {
		return err
	}
	if s.Gate != nil {
		if _, _, err := s.Gate.Execute(ctx, workflowgate.Input{
			Gate: "deployment", ContractDID: envelope.DID,
			Requester:    "system:auto-deploy",
			Continuation: map[string]any{"requested_by": "system:auto-deploy"},
		}); err != nil {
			return err
		}
	}

	_, err := s.Deployer.Handle(ctx, command.DeployCmd{
		DID:         envelope.DID,
		RequestedBy: "system:auto-deploy",
		LocalPeer:   s.LocalPeer,
		EventID:     evt.ID(),
	})
	return err
}
