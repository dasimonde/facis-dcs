package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
)

// A peer proposal whose change_request is neither a note nor a ChangeRequest
// object is refused with 400 before it is stored, so it never reaches the
// merge of the round.
func TestAShippedProposalWithAMalformedChangeRequestIsRefused(t *testing.T) {
	err := (&dcsToDcssrvc{}).persistShippedProposal(context.Background(), nil, "did:web:local",
		&dcstodcs.DCSToDCSNegotiationMessageRequest{ChangeRequest: map[string]any{"name": 7}})

	require.Equal(t, "bad_request", errorName(err))
	require.ErrorIs(t, err, negotiationmerging.ErrInvalidChangeRequest)
}

// A negotiate request with a malformed change request answers 400.
func TestAMalformedChangeRequestAnswersBadRequest(t *testing.T) {
	_, _, err := negotiationmerging.Decode([]byte(`42`))

	require.Equal(t, "bad_request", errorName(mapContractCommandError(err)))
}
