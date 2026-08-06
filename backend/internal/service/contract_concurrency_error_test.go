package service

import (
	"fmt"
	"testing"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

// A background writer — the PDF regenerator, an arriving peer ship — advances
// updated_at between a caller's read and its command. Re-reading and reissuing
// succeeds, so this is a conflict the caller resolves, not a fault. Reported as
// internal_error (500, temporary:false) it claimed the request would never
// succeed, and callers that could simply have re-read gave up instead.
func TestUpdatedElsewhereIsAConflictTheCallerCanRetry(t *testing.T) {
	err := mapContractCommandError(
		fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere))

	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "conflict", serviceErr.Name)
	require.True(t, serviceErr.Temporary)
	require.False(t, serviceErr.Fault)
}

// The remote variant asks for a peer re-sync first, but it is the same refusal
// and must not fall through to internal_error just because its guidance differs.
func TestUpdatedElsewhereFromAPeerIsTheSameConflict(t *testing.T) {
	err := mapContractCommandError(
		fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere))

	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "conflict", serviceErr.Name)
}

// A refusal the caller cannot resolve by re-reading keeps its own code.
func TestInvalidTransitionIsStillABadRequest(t *testing.T) {
	err := mapContractCommandError(
		fmt.Errorf("%w: DRAFT cannot terminate", contractstate.ErrInvalidTransition))

	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "bad_request", serviceErr.Name)
}

func TestTemplateUpdatedElsewhereIsAlsoAConflict(t *testing.T) {
	err := mapTemplateCommandError(
		fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere))

	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "conflict", serviceErr.Name)
	require.True(t, serviceErr.Temporary)
}
