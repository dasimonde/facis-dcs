package deployevent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
)

type recordingDeployer struct {
	commands []command.DeployCmd
	notReady error
}

func (r *recordingDeployer) Deployable(context.Context, string, string) error {
	return r.notReady
}

func (r *recordingDeployer) Handle(_ context.Context, cmd command.DeployCmd) (*command.DeployResult, error) {
	r.commands = append(r.commands, cmd)
	return &command.DeployResult{DID: cmd.DID}, nil
}

func appliedSignatureEvent(t *testing.T, did string) cloudevent.Event {
	t.Helper()
	evt := cloudevent.New()
	evt.SetID("1")
	evt.SetSource("test")
	evt.SetType("APPLIED_SIGNATURE")
	require.NoError(t, evt.SetData(cloudevent.ApplicationJSON, json.RawMessage(`{"did":"`+did+`"}`)))
	return evt
}

// Both deployment paths run the same multi-signer gate, and that gate reads
// which declared slot is this instance's own from LocalPeer. Left empty here,
// every slot of a federated contract looked local and the counterparty's — held
// only in the counterparty's database — was demanded from ours, so a fully
// countersigned contract could never auto-deploy at all.
func TestAutoDeployPassesThisInstancesOwnPeerIdentity(t *testing.T) {
	deployer := &recordingDeployer{}
	sub := &Subscriber{Deployer: deployer, LocalPeer: "did:web:dcs-a.localhost"}

	require.NoError(t, sub.handle(context.Background(), appliedSignatureEvent(t, "did:web:example#contract")))

	require.Len(t, deployer.commands, 1)
	require.Equal(t, "did:web:dcs-a.localhost", deployer.commands[0].LocalPeer)
	require.Equal(t, "did:web:example#contract", deployer.commands[0].DID)
	require.Equal(t, "system:auto-deploy", deployer.commands[0].RequestedBy)
}

// DCS-NFR-SF-01: the event id becomes the deployment's correlation id, so a
// redelivered event is recognized (command.ErrAlreadyDispatched) and
// acknowledged, while a failure a later attempt can overcome is redelivered.
func TestAutoDeployIsIdempotentPerEvent(t *testing.T) {
	deployer := &recordingDeployer{}
	sub := &Subscriber{Deployer: deployer}
	evt := appliedSignatureEvent(t, "did:web:example#contract")

	require.NoError(t, sub.consume(evt))
	require.Len(t, deployer.commands, 1)
	require.Equal(t, evt.ID(), deployer.commands[0].EventID)

	deployer.notReady = command.ErrAlreadyDispatched
	require.NoError(t, sub.consume(evt), "a redelivered event is acknowledged")

	deployer.notReady = errors.New("connection refused")
	require.Error(t, sub.consume(evt), "a transient failure is delivered again")
}

func TestAutoDeployIgnoresAnEventWithoutAContract(t *testing.T) {
	deployer := &recordingDeployer{}
	sub := &Subscriber{Deployer: deployer, LocalPeer: "did:web:dcs-a.localhost"}

	require.NoError(t, sub.handle(context.Background(), appliedSignatureEvent(t, "")))
	require.Empty(t, deployer.commands)
}

// After the first of two signatures the contract may not deploy yet. The
// deployment workflow gate must not be dispatched then: an open gate run
// blocks the deployment once the last signature arrives.
func TestAutoDeployDispatchesNoGateForAPartiallySignedContract(t *testing.T) {
	deployer := &recordingDeployer{notReady: command.ErrSigningIncomplete}
	// An unconfigured coordinator answers every Execute with a BlockedError.
	sub := &Subscriber{Deployer: deployer, Gate: &workflowgate.Coordinator{}, LocalPeer: "did:web:dcs-a.localhost"}

	err := sub.handle(context.Background(), appliedSignatureEvent(t, "did:web:example#contract"))

	require.ErrorIs(t, err, command.ErrSigningIncomplete)
	var blocked *workflowgate.BlockedError
	require.NotErrorAs(t, err, &blocked, "the gate ran for a contract that may not deploy")
	require.Empty(t, deployer.commands)
}
