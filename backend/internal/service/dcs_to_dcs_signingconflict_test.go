package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	"digital-contracting-service/internal/contractworkflowengine/command"
)

// ADR-38 §2: the peer gets a signing conflict as post_pdf's conflict error,
// which its synchronizer does not retry; any other receiver failure keeps its
// error.
func TestASigningConflictIsAnsweredAsAConflict(t *testing.T) {
	for _, tc := range []struct {
		err  error
		name string
	}{
		{fmt.Errorf("receive: %w", command.ErrSigningConflict), "conflict"},
		{command.ErrShippedDisagreesWithLocalRound, "bad_request"},
		{errors.New("database unavailable"), "internal_error"},
	} {
		var serviceErr *goa.ServiceError
		require.ErrorAs(t, receiverError("did:web:dcs.example:contract:x", tc.err), &serviceErr)
		require.Equal(t, tc.name, serviceErr.Name, tc.err.Error())
	}
}
