// Package deployevent auto-deploys a contract once its signing workflow
// completes (DCS-FR-CWE-06): it subscribes to the signingmanagement
// APPLIED_SIGNATURE event on the NATS event bus and calls the same
// command.Deployer the manual POST /contract/deploy endpoint uses, so both
// paths share one deployment implementation.
package deployevent

import (
	"context"
	"errors"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/command"
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
	Handle(ctx context.Context, cmd command.DeployCmd) (*command.DeployResult, error)
}

// Start registers the event handler with the NATS sub-client and begins
// consuming events. It returns immediately; the subscription runs in the
// background until the sub-client is closed.
func (s *Subscriber) Start(subClient *event.CloudEventSubClient) error {
	return subClient.Subscribe(func(evt cloudevent.Event) {
		if evt.Type() != smeventtype.Applied.String() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		// A contract that designates no target system is not an error: not every
		// party deploys what it signs — a negotiating peer may simply hold the
		// agreement. Deployment is opt-in, so this is an ordinary outcome
		// (ADR-25) and must not be reported as a failure.
		if err := s.handle(ctx, evt); err != nil && !errors.Is(err, command.ErrNoTargetDesignated) {
			log.Printf("contractworkflowengine/deployevent: could not auto-deploy: %v", err)
		}
	})
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
	})
	return err
}
