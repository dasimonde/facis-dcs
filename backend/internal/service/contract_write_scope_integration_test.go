package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/middleware"
)

// A contract mutation by an organization that may not read the contract is
// refused before any command runs. The service has no repositories here, so a
// call that passed the check would panic.
func TestContractWritesRequireReadAccessPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES ('org-a-draft',$1,'Org A','DRAFT','template',1,'draft','{}')`, peer)
	require.NoError(t, err)

	s := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *document}
	roles := []string{"Contract Creator", "Contract Negotiator", "Contract Reviewer", "Contract Approver", "Contract Manager"}
	// A Sys.* role reads every organization's contracts, but it writes only
	// for its own organization (ADR-27).
	sysRoles := []string{"Sys. Contract Creator", "Sys. Contract Reviewer", "Sys. Contract Approver", "Sys. Contract Manager"}
	for _, caller := range [][]string{roles, sysRoles} {
		refuseEveryWrite(t, s, middleware.InjectAuthContext(context.Background(), caller, "holder-c", "Org C"), "org-a-draft")
	}
}

func refuseEveryWrite(t *testing.T, s *contractWorkflowEnginesrvc, ctx context.Context, did string) {
	t.Helper()
	calls := map[string]func() error{
		"update":       func() error { _, err := s.Update(ctx, &cwe.ContractUpdateRequest{Did: did}); return err },
		"submit":       func() error { _, err := s.Submit(ctx, &cwe.ContractSubmitRequest{Did: did}); return err },
		"negotiate":    func() error { _, err := s.Negotiate(ctx, &cwe.ContractNegotiationRequest{Did: did}); return err },
		"accept_offer": func() error { _, err := s.AcceptOffer(ctx, &cwe.ContractOfferAcceptRequest{Did: did}); return err },
		"save_negotiation_draft": func() error {
			_, err := s.SaveNegotiationDraft(ctx, &cwe.ContractNegotiationDraftSaveRequest{Did: did})
			return err
		},
		"delete_negotiation_draft": func() error {
			_, err := s.DeleteNegotiationDraft(ctx, &cwe.ContractNegotiationDraftRetrieveRequest{Did: did})
			return err
		},
		"respond":   func() error { _, err := s.Respond(ctx, &cwe.ContractNegotiationRespondRequest{Did: did}); return err },
		"approve":   func() error { _, err := s.Approve(ctx, &cwe.ContractApproveRequest{Did: did}); return err },
		"reject":    func() error { _, err := s.Reject(ctx, &cwe.ContractRejectRequest{Did: did}); return err },
		"terminate": func() error { _, err := s.Terminate(ctx, &cwe.ContractTerminateRequest{Did: did}); return err },
		"renew":     func() error { _, err := s.Renew(ctx, &cwe.ContractRenewRequest{Did: did}); return err },
		"offer":     func() error { _, err := s.Offer(ctx, &cwe.ContractOfferRequest{Did: did}); return err },
		"withdraw":  func() error { _, err := s.Withdraw(ctx, &cwe.ContractWithdrawRequest{Did: did}); return err },
		"deploy":    func() error { _, err := s.Deploy(ctx, &cwe.ContractDeployRequest{Did: did}); return err },
		"designate_target": func() error {
			_, err := s.DesignateContractTarget(ctx, &cwe.ContractTargetDesignateRequest{Did: did})
			return err
		},
		"store": func() error { _, err := s.Store(ctx, &cwe.ContractStoreRequest{Did: did}); return err },
		"publish_catalogue": func() error {
			_, err := s.PublishCatalogue(ctx, &cwe.ContractCataloguePublishRequest{Did: did})
			return err
		},
	}
	for name, call := range calls {
		var refused *goa.ServiceError
		err := call()
		require.True(t, errors.As(err, &refused) && refused.Name == "forbidden", "%s by a foreign organization: %v", name, err)
	}
}

// DCS-FR-CWE-07: an organization named in dcs:parties reads the contract, but
// only the creating organization edits the draft (as on submit) and registers
// it in the catalogue. A system client acting for this instance writes.
func TestContractWritesAreBoundToTheOrganizationPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data,updated_at)
		VALUES ('a-draft',$1,'Org A','DRAFT','template',1,'original','{"dcs:parties":[{"dcs:legalName":"Org A"},{"dcs:legalName":"Org B"}]}', now() - interval '1 hour')`, peer)
	require.NoError(t, err)

	orgB := middleware.InjectAuthContext(context.Background(), []string{"Contract Creator", "Contract Approver"}, "holder-b", "Org B")
	require.NoError(t, authorizeContractWrite(orgB, database, *document, "a-draft"), "a named party may act on the contract")
	require.Equal(t, "forbidden", errorName(authorizeContractOwner(orgB, database, *document, "a-draft")), "a party that did not create it may not publish it")
	instance := middleware.InjectAuthContext(context.Background(), []string{"Sys. Contract Approver"}, "", peer)
	require.NoError(t, authorizeContractWrite(instance, database, *document, "a-draft"))
	require.NoError(t, authorizeContractOwner(instance, database, *document, "a-draft"))

	name := "rewritten by Org B"
	updater := command.Updater{DB: database, CRepo: &cwepg.PostgresContractRepo{}, DIDDocument: *document}
	err = updater.Handle(orgB, command.UpdateCmd{DID: "a-draft", UpdatedAt: time.Now().Add(time.Hour), UpdatedBy: "Org B", Name: &name, CauserDID: peer})
	require.ErrorIs(t, err, command.ErrNotCreator)
	require.Equal(t, "forbidden", errorName(mapContractCommandError(err)))
	var got string
	require.NoError(t, database.Get(&got, `SELECT name FROM contracts WHERE did='a-draft'`))
	require.Equal(t, "original", got)
}

// A malformed updated_at is the caller's error: 400, not 500.
func TestMalformedUpdatedAtIsABadRequestPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES ('a',$1,'Org A','ACTIVE','template',1,'a','{}')`, peer)
	require.NoError(t, err)
	s := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *document}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Contract Creator", "Contract Approver", "Contract Manager"}, "holder-a", "Org A")
	calls := map[string]func() error{
		"update": func() error {
			_, err := s.Update(ctx, &cwe.ContractUpdateRequest{Did: "a", UpdatedAt: "yesterday"})
			return err
		},
		"terminate": func() error {
			_, err := s.Terminate(ctx, &cwe.ContractTerminateRequest{Did: "a", UpdatedAt: "yesterday", Reason: "r"})
			return err
		},
		"approve": func() error {
			_, err := s.Approve(ctx, &cwe.ContractApproveRequest{Did: "a", UpdatedAt: "yesterday"})
			return err
		},
		"deploy": func() error {
			_, err := s.Deploy(ctx, &cwe.ContractDeployRequest{Did: "a", UpdatedAt: "yesterday"})
			return err
		},
	}
	for name, call := range calls {
		err := call()
		require.Equal(t, "bad_request", errorName(err), "%s: %v", name, err)
		require.ErrorContains(t, err, "parsing time", name)
	}
}
