package service

import (
	"fmt"
	"testing"

	"digital-contracting-service/internal/contractworkflowengine/command"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

func TestInvalidOriginatorRoleIsBadRequest(t *testing.T) {
	err := mapContractCommandError(fmt.Errorf("create contract: %w", command.ErrInvalidOriginatorRole))
	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "bad_request", serviceErr.Name)
}
