package deployevent

import (
	"context"
	"encoding/json"
	"testing"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/command"
)

type recordingDeployer struct {
	commands []command.DeployCmd
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

func TestAutoDeployIgnoresAnEventWithoutAContract(t *testing.T) {
	deployer := &recordingDeployer{}
	sub := &Subscriber{Deployer: deployer, LocalPeer: "did:web:dcs-a.localhost"}

	require.NoError(t, sub.handle(context.Background(), appliedSignatureEvent(t, "")))
	require.Empty(t, deployer.commands)
}
