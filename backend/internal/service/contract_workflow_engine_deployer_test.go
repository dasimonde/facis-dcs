package service

import (
	"testing"

	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"

	"github.com/stretchr/testify/require"
)

func TestWorkflowDeploymentHandlersSharePeerSignatureStore(t *testing.T) {
	syncRepo := syncpg.PostgresSyncRepository{}
	service := &contractWorkflowEnginesrvc{SRepo: syncRepo}

	require.Equal(t, service.SRepo, service.deployer().PeerSigs)
}
