package service

import (
	"errors"
	"fmt"
	"testing"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/command"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

// A signer must be able to tell "the contract is waiting for the counterparty
// to settle this version" from "you may not sign this contract". Both are
// client errors, so both answer 400 and neither can be distinguished by
// status; the frontend reads the code, which is why the settlement refusal
// carries one of its own instead of sharing bad_request.
func TestCounterpartyNotSettledIsItsOwnAPICode(t *testing.T) {
	notSettled := mapSignatureCommandError(
		fmt.Errorf("%w: no settlement from %s is held", command.ErrCounterpartyNotSettled, "did:web:dcs-b.localhost"))

	var settlementErr *goa.ServiceError
	require.ErrorAs(t, notSettled, &settlementErr)
	require.Equal(t, "counterparty_not_settled", settlementErr.Name)
	require.Contains(t, settlementErr.Message, "did:web:dcs-b.localhost")

	mayNotSign := mapSignatureCommandError(
		fmt.Errorf("%w: DRAFT cannot sign", contractstate.ErrInvalidTransition))

	var transitionErr *goa.ServiceError
	require.ErrorAs(t, mayNotSign, &transitionErr)
	require.Equal(t, "bad_request", transitionErr.Name)
	require.NotEqual(t, settlementErr.Name, transitionErr.Name)
}

// The settlement gate hard-fails when its store is missing rather than waving
// the signature through; that is an operator fault, not something a signer can
// resolve by waiting, so it must not borrow the waiting-for-the-counterparty
// code.
func TestMissingSettlementStoreIsNotReportedAsWaiting(t *testing.T) {
	err := mapSignatureCommandError(errors.New("could not check the counterparty settlement of did:web:x: no settlement store is configured"))

	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "internal_error", serviceErr.Name)
}
