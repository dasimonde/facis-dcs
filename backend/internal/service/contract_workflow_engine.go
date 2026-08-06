package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"time"

	"digital-contracting-service/internal/dcstodcs"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/semantichub"

	contracttemplate2 "digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/tsa"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationactionflag"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/middleware"
	qry2 "digital-contracting-service/internal/processauditandcompliance/query"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"

	"github.com/jmoiron/sqlx"
	goa "goa.design/goa/v3/pkg"
)

type contractWorkflowEnginesrvc struct {
	DB                   *sqlx.DB
	CRepo                db.ContractRepo
	RTRepo               db.ReviewTaskRepo
	ATRepo               db.ApprovalTaskRepo
	NTRepo               db.NegotiationTaskRepo
	NRepo                db.NegotiationRepo
	SRepo                db2.SyncRepository
	CTRepo               db.ContractTemplateRepo
	DeploymentRepo       db.DeploymentRepo
	TargetRepo           db.ContractTargetRepo
	FCClient             *fcclient.FederatedCatalogueClient
	DIDDocument          identity.DIDDocument
	ATrailReader         base.AuditTrailReader
	DCSToDCSSynchronizer dcstodcs.DCSToDCSSynchronizer
	TrustPool            *identity.EUTrustPool
	ArchiveNotary        command.ArchiveNotary
	ArchiveTSA           *tsa.APIClient
	TargetClient         command.ContractTargetClient
	WorkflowGate         *workflowgate.Coordinator
	// MachineIdentities is the registry every machine caller is resolved
	// against, and HydraAdmin provisions the OAuth2 clients they present
	// (ADR-27).
	MachineIdentities    machineidentity.Repo
	HydraAdmin           MachineCredentialIssuer
	HydraPublicIssuerURL string
	auth.JWTAuthenticator
}

func NewContractWorkflowEngine(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo,
	sRepo db2.SyncRepository, trustPool *identity.EUTrustPool,
	fcClient *fcclient.FederatedCatalogueClient, auditTrailReader base.AuditTrailReader, didDocument identity.DIDDocument,
	archiveNotary command.ArchiveNotary, archiveTSA *tsa.APIClient,
	deploymentRepo db.DeploymentRepo, targetRepo db.ContractTargetRepo,
	targetClient command.ContractTargetClient,
	workflowGate *workflowgate.Coordinator,
	machineIdentities machineidentity.Repo, hydraAdmin MachineCredentialIssuer,
	hydraPublicIssuerURL string) contractworkflowengine.Service {

	service := &contractWorkflowEnginesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		SRepo:            sRepo,
		CTRepo:           ctRepo,
		DeploymentRepo:   deploymentRepo,
		TargetRepo:       targetRepo,
		FCClient:         fcClient,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
		TrustPool:        trustPool,
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
		TargetClient:     targetClient,
		WorkflowGate:     workflowGate,

		MachineIdentities:    machineIdentities,
		HydraAdmin:           hydraAdmin,
		HydraPublicIssuerURL: hydraPublicIssuerURL,
	}
	if workflowGate != nil {
		for _, gate := range []string{"submission", "offer", "approval", "deployment"} {
			workflowGate.SetReviewContinuation(gate, service.resumeReviewedWorkflowGate)
		}
	}
	return service
}

func workflowRoles(ctx context.Context) []string {
	roles := middleware.GetUserRoles(ctx)
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.String())
	}
	return result
}

func (s *contractWorkflowEnginesrvc) runWorkflowGate(ctx context.Context, gate, did string, updatedAt time.Time, continuation map[string]any) (time.Time, bool, error) {
	_, reused, snapshotUpdatedAt, err := s.WorkflowGate.ExecuteSnapshot(ctx, workflowgate.Input{
		Gate: gate, ContractDID: did, ExpectedUpdatedAt: updatedAt,
		Requester: middleware.GetParticipantID(ctx), Roles: workflowRoles(ctx),
		Continuation: continuation,
	})
	return snapshotUpdatedAt, reused, err
}

func (s *contractWorkflowEnginesrvc) resumeReviewedWorkflowGate(ctx context.Context, run workflowgate.Run) error {
	stringValue := func(name string) string {
		value, _ := run.Continuation[name].(string)
		return value
	}
	roles := userrole.UserRoles{}
	if values, ok := run.Continuation["user_roles"].([]any); ok {
		for _, value := range values {
			if role, ok := value.(string); ok {
				roles = append(roles, userrole.UserRole(role))
			}
		}
	}
	switch run.Gate {
	case "submission":
		handler := command.Submitter{
			DB: s.DB, CRepo: s.CRepo, RTRepo: s.RTRepo, ATRepo: s.ATRepo,
			NRepo: s.NRepo, NTRepo: s.NTRepo, SRepo: s.SRepo, DIDDocument: s.DIDDocument,
		}
		return handler.Handle(ctx, command.SubmitCmd{
			DID: run.ContractDID, UpdatedAt: run.ContractUpdatedAt,
			SubmittedBy: stringValue("requested_by"), HolderDID: stringValue("holder_did"),
			UserRoles: roles, CauserDID: stringValue("causer_did"),
		})
	case "offer":
		return (&command.Offerer{DB: s.DB, CRepo: s.CRepo, DIDDocument: s.DIDDocument}).Handle(ctx, command.OfferCmd{
			DID: run.ContractDID, UpdatedAt: run.ContractUpdatedAt,
			OfferedBy: stringValue("requested_by"), HolderDID: stringValue("holder_did"),
			UserRoles: roles, CauserDID: stringValue("causer_did"),
		})
	case "approval":
		return (&command.Approver{
			DB: s.DB, CRepo: s.CRepo, ATRepo: s.ATRepo, SRepo: s.SRepo, DIDDocument: s.DIDDocument,
		}).Handle(ctx, command.ApproveCmd{
			DID: run.ContractDID, UpdatedAt: run.ContractUpdatedAt,
			ApprovedBy: stringValue("requested_by"), HolderDID: stringValue("holder_did"),
			UserRoles: roles, CauserDID: stringValue("causer_did"),
		})
	case "deployment":
		_, err := (&command.Deployer{
			DB: s.DB, CRepo: s.CRepo, DeploymentRepo: s.DeploymentRepo,
			TargetRepo: s.TargetRepo, Target: s.TargetClient, PeerSigs: s.SRepo,
		}).Handle(ctx, command.DeployCmd{
			DID: run.ContractDID, UpdatedAt: run.ContractUpdatedAt,
			RequestedBy: stringValue("requested_by"), LocalPeer: stringValue("causer_did"),
			TargetIDOverride: stringValue("target_id"),
		})
		return err
	default:
		return fmt.Errorf("reviewed continuation is not available for gate %q", run.Gate)
	}
}

// mapContractCommandError classifies a contract command handler error for
// the HTTP layer: state-machine transition failures (contractstate.
// ErrInvalidTransition, the single source of truth introduced by the
// contract-state-machine-refactor) are client errors (400), everything else
// remains an internal error (500).
func mapContractCommandError(err error) error {
	if err == nil {
		return nil
	}
	// A background writer — the PDF regenerator, an arriving peer ship —
	// advanced updated_at between the caller's read and its command. Re-reading
	// and reissuing succeeds, so this answers 409 with temporary set, where
	// internal_error told the caller its request would never succeed.
	if errors.Is(err, base.ErrUpdatedElsewhere) {
		return goa.NewServiceError(err, "conflict", false, true, false)
	}
	if errors.Is(err, contractstate.ErrInvalidTransition) ||
		errors.Is(err, validation.ErrContractHierarchyInvalid) ||
		errors.Is(err, validation.ErrContractNotClosed) ||
		errors.Is(err, command.ErrContractHierarchyCycle) ||
		errors.Is(err, command.ErrInvalidOriginatorRole) ||
		errors.Is(err, command.ErrDeploymentNotFound) ||
		errors.Is(err, command.ErrKPIVerdictUnknown) ||
		errors.Is(err, command.ErrKPIRuleMissing) ||
		errors.Is(err, command.ErrKPIRuleUnknown) ||
		// Deployment refusals are the operator's to fix — no target designated,
		// one that is not registered, one that is disabled. Returning 500 made
		// each read as an outage rather than as the answer to what was asked.
		errors.Is(err, command.ErrNoTargetDesignated) ||
		errors.Is(err, command.ErrTargetNotRegistered) ||
		errors.Is(err, command.ErrTargetDisabled) ||
		errors.Is(err, command.ErrSigningIncomplete) ||
		errors.Is(err, command.ErrContractNotRenewable) ||
		errors.Is(err, command.ErrNotAParty) ||
		errors.Is(err, command.ErrConflictOfInterest) ||
		errors.Is(err, command.ErrAgreementSettled) ||
		errors.Is(err, command.ErrOwnAgreementSettled) ||
		errors.Is(err, command.ErrNegotiationNotSettled) ||
		errors.Is(err, db.ErrNoMatchingDecision) {
		return contractworkflowengine.MakeBadRequest(err)
	}
	// A contract whose own ODRL policies are not satisfied is a rejected
	// request, not a server fault: the caller asked to approve a contract whose
	// reported values violate its agreed boundaries, and the finding already
	// names the rule and the comparison. Returning 500 made a legitimate policy
	// refusal read as an outage.
	var policyErr validation.ContractPolicySatisfactionError
	if errors.As(err, &policyErr) {
		return contractworkflowengine.MakeBadRequest(err)
	}
	return contractworkflowengine.MakeInternalError(err)
}

func (s *contractWorkflowEnginesrvc) Create(ctx context.Context, req *contractworkflowengine.ContractCreateRequest) (res *contractworkflowengine.ContractCreateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	did, err := base.GenerateID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	counterparty := ""
	if req.Counterparty != nil {
		counterparty = *req.Counterparty
	}
	// A counterparty is not vetted at creation time (ADR-19): the federation
	// trust gate — agreement credential + local policy endpoint — is
	// consulted once, on the actual ship attempt (dcstodcs.DCSToDCSSynchronizer.
	// shipContractPDF), so a denial surfaces as a sync_fails/incident there,
	// not as a create-time rejection.

	cmd := command.CreateCmd{
		DID:          *did,
		TemplateDID:  req.TemplateDid,
		CreatedBy:    middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		Counterparty: counterparty,
		Parties:      req.Parties,
		OriginatorRole: func() string {
			if req.OriginatorRole != nil {
				return *req.OriginatorRole
			}
			return ""
		}(),
	}
	createHandler := command.Creator{
		DB:          s.DB,
		CTRepo:      s.CTRepo,
		CRepo:       s.CRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		NTRepo:      s.NTRepo,
		DIDDocument: s.DIDDocument,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractCreateResponse{
		Did: *did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Update(ctx context.Context, req *contractworkflowengine.ContractUpdateRequest) (res *contractworkflowengine.ContractUpdateResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contractData, err := datatype.NewJSON(req.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var startDate *time.Time
	if req.StartDate != nil {
		startD, err := time.Parse(time.RFC3339, *req.StartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		startDate = &startD
	}

	var expDate *time.Time
	if req.ExpDate != nil {
		expD, err := time.Parse(time.RFC3339, *req.ExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expDate = &expD
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if req.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*req.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.UpdateCmd{
		DID:             req.Did,
		UpdatedAt:       updatedAt,
		UpdatedBy:       middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
		UserRoles:       middleware.GetUserRoles(ctx),
		Name:            req.Name,
		Description:     req.Description,
		ContractData:    &contractData,
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: req.ExpNoticePeriod,
		CauserDID:       localPeer,
	}
	handler := command.Updater{
		DB:          s.DB,
		CRepo:       s.CRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		NTRepo:      s.NTRepo,
		NRepo:       s.NRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractUpdateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Submit(ctx context.Context, req *contractworkflowengine.ContractSubmitRequest) (res *contractworkflowengine.ContractSubmitResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var actionFlag *actionflag.ActionFlag
	if req.ForwardTo != nil {
		flag, err := actionflag.NewActionFlag(*req.ForwardTo)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		actionFlag = &flag
	}

	var contractData *datatype.JSON
	if req.ContractData != nil {
		data, err := datatype.NewJSON(req.ContractData)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		contractData = &data
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	gateUpdatedAt, reusedGate, err := s.runWorkflowGate(ctx, "submission", req.Did, updatedAt, map[string]any{
		"requested_by": middleware.GetParticipantID(ctx),
		"holder_did":   middleware.GetHolderDID(ctx),
		"user_roles":   workflowRoles(ctx),
		"causer_did":   localPeer,
	})
	if err != nil {
		return nil, err
	}
	updatedAt = gateUpdatedAt
	if reusedGate {
		qryHandler := contract.GetProcessDataByIDHandler{DB: s.DB, CRepo: s.CRepo}
		processData, readErr := qryHandler.Handle(ctx, contract.GetProcessDataByIDQry{
			DID: req.Did, RetrievedBy: middleware.GetParticipantID(ctx),
			HolderDID: middleware.GetHolderDID(ctx),
		})
		if readErr == nil && processData.UpdatedAt.After(updatedAt) {
			return &contractworkflowengine.ContractSubmitResponse{Did: req.Did, CurrentState: processData.State.String()}, nil
		}
	}

	cmd := command.SubmitCmd{
		DID:          req.Did,
		UpdatedAt:    updatedAt,
		SubmittedBy:  middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		ActionFlag:   actionFlag,
		Comments:     req.Comments,
		ContractData: contractData,
		CauserDID:    localPeer,
	}
	handler := command.Submitter{
		DB:          s.DB,
		CRepo:       s.CRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		NRepo:       s.NRepo,
		NTRepo:      s.NTRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	qry := contract.GetProcessDataByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
	}
	qryHandler := contract.GetProcessDataByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	processData, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractSubmitResponse{
		Did:          req.Did,
		CurrentState: processData.State.String(),
	}, nil
}

func (s *contractWorkflowEnginesrvc) Retrieve(ctx context.Context, req *contractworkflowengine.ContractRetrieveRequest) (res *contractworkflowengine.ContractRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contract.GetAllMetadataQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		ParentDID:   base.DerefString(req.ParentDid),
		Pagination:  pagination,
		DIDDocument: s.DIDDocument,
	}
	qryHandler := contract.GetAllMetadataHandler{
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractItem
	for _, item := range result.Contracts {
		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			p, err := expirationpolicy.NewExpirationPolicy(*item.ExpPolicy)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			s := p.String()
			expPolicy = &s
		}

		state, err := contractstate.NewContractState(item.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		contracts = append(contracts, &contractworkflowengine.ContractItem{
			Did:                  item.DID,
			ContractVersion:      item.ContractVersion,
			State:                state.String(),
			Name:                 item.Name,
			Description:          item.Description,
			CreatedBy:            item.CreatedBy,
			CreatedAt:            item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:            item.UpdatedAt.Format(time.RFC3339),
			TemplateDid:          item.TemplateDID,
			TemplateVersion:      item.TemplateVersion,
			StartDate:            startDate,
			ExpDate:              expDate,
			ExpPolicy:            expPolicy,
			ExpNoticePeriod:      item.ExpNoticePeriod,
			Responsible:          item.Responsible,
			LatestTemplateDid:    item.LatestTemplateDID,
			TemplateIsDeprecated: item.TemplateIsDeprecated,
			ParentContractDid:    item.ParentContractDID,
			TargetID:             item.TargetID,
			TargetName:           item.TargetName,
		})
	}

	var reviewTasks []*contractworkflowengine.ContractReviewTaskItem
	for _, item := range result.ReviewerTasks {
		reviewTasks = append(reviewTasks, &contractworkflowengine.ContractReviewTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			Reviewer:        item.Reviewer,
			State:           item.State.String(),
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	var approvalTasks []*contractworkflowengine.ContractApprovalTaskItem
	for _, item := range result.ApprovalTasks {
		approvalTasks = append(approvalTasks, &contractworkflowengine.ContractApprovalTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Approver:        item.Approver,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	var negotiationTasks []*contractworkflowengine.ContractNegotiationTaskItem
	for _, item := range result.NegotiatorTasks {
		negotiationTasks = append(negotiationTasks, &contractworkflowengine.ContractNegotiationTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Negotiator:      item.Negotiator,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	return &contractworkflowengine.ContractRetrieveResponse{
		Contracts:        contracts,
		ReviewTasks:      reviewTasks,
		ApprovalTasks:    approvalTasks,
		NegotiationTasks: negotiationTasks,
	}, nil
}

// supersessionItems reads back the annotation the negotiation merge left on a
// change request it accepted and then discarded (last-accepted-wins). A reader
// of the contract otherwise sees only the ACCEPTED decision and would take the
// request's content for part of the agreement.
func supersessionItems(annotation *datatype.JSON) ([]*contractworkflowengine.ContractNegotiationSupersessionItem, error) {
	if annotation == nil {
		return nil, nil
	}
	var records []db.NegotiationSupersession
	if err := json.Unmarshal(*annotation, &records); err != nil {
		return nil, fmt.Errorf("could not read superseded change request record: %w", err)
	}
	items := make([]*contractworkflowengine.ContractNegotiationSupersessionItem, 0, len(records))
	for _, record := range records {
		items = append(items, &contractworkflowengine.ContractNegotiationSupersessionItem{
			SupersededBy: record.SupersededByID,
			Fields:       record.Fields,
		})
	}
	return items, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveByID(ctx context.Context, req *contractworkflowengine.ContractRetrieveByIDRequest) (res *contractworkflowengine.ContractRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	qry := contract.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		LocalPeer:   localPeer,
	}
	qryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
		NRepo: s.NRepo,
	}
	contractResult, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		if errors.Is(err, contract.ErrContractAccessDenied) {
			return nil, contractworkflowengine.MakeForbidden(err)
		}
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	negotiations := make(map[string]*contractworkflowengine.ContractNegotiationItem)
	for _, item := range contractResult.Negotiations {
		negotiation, ok := negotiations[item.ID]
		if !ok {
			superseded, err := supersessionItems(item.SupersededBy)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			negotiation = &contractworkflowengine.ContractNegotiationItem{
				ID:              item.ID,
				ContractVersion: item.ContractVersion,
				ChangeRequest:   item.ChangeRequest,
				CreatedBy:       item.CreatedBy,
				CreatedAt:       item.CreatedAt.String(),
				Superseded:      superseded,
			}
			negotiations[item.ID] = negotiation
		}

		negotiation.NegotiationDecisions = append(negotiation.NegotiationDecisions, &contractworkflowengine.ContractNegotiationDecisionItem{
			Negotiator:      item.Negotiator,
			Decision:        item.Decision,
			RejectionReason: item.RejectionReason,
		})
	}

	negotiationList := slices.Collect(maps.Values(negotiations))

	var startDate *string
	if contractResult.StartDate != nil {
		s := contractResult.StartDate.Format(time.RFC3339)
		startDate = &s
	}

	var expDate *string
	if contractResult.ExpDate != nil {
		s := contractResult.ExpDate.Format(time.RFC3339)
		expDate = &s
	}

	var expPolicy *string
	if contractResult.ExpPolicy != nil {
		s := contractResult.ExpPolicy.String()
		expPolicy = &s
	}

	kpis, err := s.retrieveKPIs(ctx, req.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	// The designated target's name, so the contract view can say where it
	// deploys to without a second round trip (ADR-25).
	var targetName *string
	if contractResult.TargetID != nil {
		targetTx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		target, err := s.TargetRepo.ReadTarget(ctx, targetTx, *contractResult.TargetID)
		_ = targetTx.Rollback()
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		if target != nil {
			targetName = &target.Name
		}
	}

	evidence, err := s.signatureEvidence(ctx, req.Did, localPeer, contractResult)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	extrinsic := string(contractstate.InferExtrinsic(contractResult.State.String(), evidence))
	return &contractworkflowengine.ContractRetrieveByIDResponse{
		Did:                contractResult.DID,
		ContractVersion:    contractResult.ContractVersion,
		State:              contractResult.State.String(),
		ExtrinsicLifecycle: &extrinsic,
		Name:               contractResult.Name,
		Description:        contractResult.Description,
		CreatedBy:          contractResult.CreatedBy,
		CreatedAt:          contractResult.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          contractResult.UpdatedAt.Format(time.RFC3339),
		ContractData:       contractResult.ContractData,
		TemplateDid:        contractResult.TemplateDID,
		TemplateVersion:    contractResult.TemplateVersion,
		Negotiations:       negotiationList,
		StartDate:          startDate,
		ExpDate:            expDate,
		ExpPolicy:          expPolicy,
		ExpNoticePeriod:    contractResult.ExpNoticePeriod,
		Responsible:        contractResult.Responsible,
		Kpis:               kpis,
		TargetID:           contractResult.TargetID,
		TargetName:         targetName,
	}, nil
}

// signatureEvidence collects who has signed a contract as far as this instance
// can evidence it: the fields the document declares, the ones carrying a local
// SIGNED signature row, and the peer this instance holds a verified
// cross-instance signature from. The extrinsic projection needs this to report
// an agreement executed only once every declared signature is collected
// (DCS-FR-SM-10) rather than on the first local one.
func (s *contractWorkflowEnginesrvc) signatureEvidence(ctx context.Context, did, localPeer string, result *contract.GetByIDResult) (contractstate.SignatureEvidence, error) {
	evidence := contractstate.SignatureEvidence{LocalPeer: localPeer}
	if result.ContractData == nil || !result.ContractData.IsNotNullValue() {
		return evidence, nil
	}
	evidence.Declared = validation.RequiredSignatureFields([]byte(*result.ContractData))
	if len(evidence.Declared) == 0 {
		return evidence, nil
	}
	if result.Responsible != nil {
		evidence.Parties = []string{result.Responsible.Creator, result.Responsible.Counterparty}
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return evidence, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	signed, err := s.CRepo.ReadSignedSignatureFieldNames(ctx, tx, did)
	if err != nil {
		return evidence, fmt.Errorf("could not read signed signature fields: %w", err)
	}
	evidence.SignedLocally = signed

	peerSig, err := s.SRepo.GetSyncSignature(ctx, tx, did)
	if err != nil {
		return evidence, fmt.Errorf("could not read the counterparty signature: %w", err)
	}
	if peerSig != nil {
		evidence.PeerSigners = []string{peerSig.FromPeerDID}
	}
	return evidence, nil
}

// KpiObservations serves the reported KPI values as a JSON-LD observation
// set: dcs:KPIObservation nodes anchored to the Semantic Hub's versioned
// context, each naming the observed metric, value, time, violation
// verdict, and the contract it observes (DCS-FR-CWE-09/-31).
func (s *contractWorkflowEnginesrvc) KpiObservations(ctx context.Context, req *contractworkflowengine.ContractRetrieveByIDRequest) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	entries, err := s.DeploymentRepo.ReadKPIsByDID(ctx, tx, req.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(fmt.Errorf("could not read KPIs for contract %s: %w", req.Did, err))
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	contextVersion, err := semantichub.ActiveVersion(ctx, s.DB, semantichub.ContextName, "context")
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(fmt.Errorf("load active hub context version: %w", err))
	}

	observations := make([]any, 0, len(entries))
	for _, entry := range entries {
		observation := map[string]any{
			"@id":               fmt.Sprintf("%s#kpi-%d", base.ResourceIRI("contract", req.Did), entry.ID),
			"@type":             "dcs:KPIObservation",
			"dcs:metricName":    entry.Metric,
			"dcs:observedValue": entry.Value,
			"dcs:observedAt":    entry.ObservedAt.Format(time.RFC3339),
			"dcs:verdict":       entry.Verdict,
			"dcs:aboutContract": map[string]any{"@id": base.ResourceIRI("contract", req.Did)},
		}
		// The rule is a node reference, not a literal: it is the @id the ODRL
		// rule carries inside the contract the target system was deployed, so a
		// reader follows it back to the exact term the verdict is about.
		if entry.RuleID != nil {
			observation["dcs:aboutRule"] = map[string]any{"@id": *entry.RuleID}
		}
		observations = append(observations, observation)
	}
	return map[string]any{
		"@context":        semantichub.AnchorURL("context", semantichub.ContextName, contextVersion),
		"@id":             req.Did + "#kpi-observations",
		"@type":           "dcs:KPIObservationSet",
		"dcs:observation": observations,
	}, nil
}

// retrieveKPIs reads the KPI reports received via deployment callbacks for a
// contract (DCS-FR-CWE-31, DCS-FR-CWE-09), each with the verdict the target
// system reached and the ODRL rule it named (ADR-33).
func (s *contractWorkflowEnginesrvc) retrieveKPIs(ctx context.Context, did string) ([]*contractworkflowengine.ContractDeploymentKPIItem, error) {
	if s.DeploymentRepo == nil {
		return nil, nil
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := s.DeploymentRepo.ReadKPIsByDID(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read KPIs for contract %s: %w", did, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	kpis := make([]*contractworkflowengine.ContractDeploymentKPIItem, 0, len(entries))
	for _, entry := range entries {
		kpis = append(kpis, &contractworkflowengine.ContractDeploymentKPIItem{
			Metric:     entry.Metric,
			Value:      entry.Value,
			ObservedAt: entry.ObservedAt.Format(time.RFC3339),
			Verdict:    entry.Verdict,
			Rule:       entry.RuleID,
		})
	}

	return kpis, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveHistoryByID(ctx context.Context, req *contractworkflowengine.ContractHistoryRetrieveByIDRequest) (res []*contractworkflowengine.ContractHistoryRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetHistoryByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	qryHandler := contract.GetHistoryByIDHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractHistoryRetrieveByIDResponse
	for _, item := range result {

		var startDate *string
		if item.StartDate != nil {
			s := item.StartDate.Format(time.RFC3339)
			startDate = &s
		}

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &contractworkflowengine.ContractHistoryRetrieveByIDResponse{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			TemplateDid:     item.TemplateDID,
			TemplateVersion: item.TemplateVersion,
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
			ContractData:    item.ContractData,
		})
	}

	return contracts, nil
}

func (s *contractWorkflowEnginesrvc) Negotiate(ctx context.Context, req *contractworkflowengine.ContractNegotiationRequest) (res *contractworkflowengine.ContractNegotiationResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	changeRequest, err := datatype.NewJSON(req.ChangeRequest)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.NegotiationCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		NegotiatedBy:  middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		ChangeRequest: &changeRequest,
		UserRoles:     middleware.GetUserRoles(ctx),
		CauserDID:     localPeer,
	}
	handler := command.Negotiator{
		DB:          s.DB,
		CRepo:       s.CRepo,
		NRepo:       s.NRepo,
		RTRepo:      s.RTRepo,
		NTRepo:      s.NTRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractNegotiationResponse{
		Did: req.Did,
	}, nil
}

// AcceptOffer takes an inbound offer into negotiation unchanged. Not to be
// confused with Respond(action_flag=ACCEPTING), which decides one already
// proposed change request.
func (s *contractWorkflowEnginesrvc) AcceptOffer(ctx context.Context, req *contractworkflowengine.ContractOfferAcceptRequest) (res *contractworkflowengine.ContractOfferAcceptResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	handler := command.OfferAcceptor{
		DB:          s.DB,
		CRepo:       s.CRepo,
		NTRepo:      s.NTRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, command.AcceptOfferCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		AcceptedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		CauserDID:  localPeer,
	})
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractOfferAcceptResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) SaveNegotiationDraft(ctx context.Context, req *contractworkflowengine.ContractNegotiationDraftSaveRequest) (res *contractworkflowengine.ContractNegotiationDraftResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	changeRequest, err := datatype.NewJSON(req.ChangeRequest)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	handler := command.NegotiationDraftSaver{
		DB:    s.DB,
		CRepo: s.CRepo,
		NRepo: s.NRepo,
	}
	// Drafts are scoped to the PARTY (participant ID): any authorized
	// negotiator of the same party continues the party's staged position;
	// nothing reaches the counterparty until proposed.
	err = handler.Handle(ctx, command.SaveNegotiationDraftCmd{
		DID:           req.Did,
		SavedBy:       middleware.GetParticipantID(ctx),
		ChangeRequest: &changeRequest,
		UserRoles:     middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractNegotiationDraftResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveNegotiationDraft(ctx context.Context, req *contractworkflowengine.ContractNegotiationDraftRetrieveRequest) (res *contractworkflowengine.ContractNegotiationDraftResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}()

	draft, err := s.NRepo.ReadDraft(ctx, tx, req.Did, middleware.GetParticipantID(ctx))
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	res = &contractworkflowengine.ContractNegotiationDraftResponse{Did: req.Did}
	if draft != nil && draft.ChangeRequest != nil {
		var changeRequest any
		if err := json.Unmarshal(*draft.ChangeRequest, &changeRequest); err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		res.ChangeRequest = changeRequest
		updatedAt := draft.UpdatedAt.Format(time.RFC3339)
		res.UpdatedAt = &updatedAt
	}
	return res, nil
}

func (s *contractWorkflowEnginesrvc) DeleteNegotiationDraft(ctx context.Context, req *contractworkflowengine.ContractNegotiationDraftRetrieveRequest) (res *contractworkflowengine.ContractNegotiationDraftResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := command.NegotiationDraftDeleter{
		DB:    s.DB,
		NRepo: s.NRepo,
	}
	err = handler.Handle(ctx, command.DeleteNegotiationDraftCmd{
		DID:       req.Did,
		SavedBy:   middleware.GetParticipantID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractNegotiationDraftResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Respond(ctx context.Context, req *contractworkflowengine.ContractNegotiationRespondRequest) (res *contractworkflowengine.ContractNegotiationRespondResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	actionFlag, err := negotiationactionflag.NewNegotiationActionFlag(req.ActionFlag)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("unknown action flag: %s (expected ACCEPTING | REJECTING)", req.ActionFlag))
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	switch actionFlag {
	case negotiationactionflag.Accepting:
		cmd := command.AcceptNegotiationCmd{
			ID:         req.ID,
			DID:        req.Did,
			AcceptedBy: middleware.GetParticipantID(ctx),
			UserRoles:  middleware.GetUserRoles(ctx),
			CauserDID:  localPeer,
		}
		handler := command.NegotiationAcceptor{
			DB:          s.DB,
			CRepo:       s.CRepo,
			NRepo:       s.NRepo,
			NTRepo:      s.NTRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			return nil, mapContractCommandError(err)
		}
	case negotiationactionflag.Rejecting:
		cmd := command.RejectNegotiationCmd{
			ID:              req.ID,
			DID:             req.Did,
			RejectedBy:      middleware.GetParticipantID(ctx),
			UserRoles:       middleware.GetUserRoles(ctx),
			RejectionReason: req.RejectionReason,
			CauserDID:       localPeer,
		}
		handler := command.NegotiationRejector{
			DB:          s.DB,
			CRepo:       s.CRepo,
			NRepo:       s.NRepo,
			NTRepo:      s.NTRepo,
			SRepo:       s.SRepo,
			DIDDocument: s.DIDDocument,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			return nil, mapContractCommandError(err)
		}
	}

	return &contractworkflowengine.ContractNegotiationRespondResponse{
		ID: req.ID,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Review(ctx context.Context, req *contractworkflowengine.ContractReviewRequest) (res *contractworkflowengine.ContractReviewResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	cmd := command.ReviewCmd{
		DID:        req.Did,
		ReviewedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := command.Reviewer{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractReviewResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Search(ctx context.Context, req *contractworkflowengine.ContractSearchRequest) (res []*contractworkflowengine.ContractSearchResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contractstate.ContractState
	if req.State != nil {
		tState, err := contractstate.NewContractState(*req.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		state = &tState
	}

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contract.GetAllMetadataByFilterQry{
		DID:             base.DerefString(req.Did),
		ContractVersion: base.DerefInt(req.ContractVersion),
		State:           state,
		RetrievedBy:     middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
		UserRoles:       middleware.GetUserRoles(ctx),
		Name:            base.DerefString(req.Name),
		Description:     base.DerefString(req.Description),
		ContractData:    base.DerefString(req.ContractData),
		ParentDID:       base.DerefString(req.ParentDid),
		Pagination:      pagination,
	}
	qryHandler := contract.GetAllMetaDataByFilterHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := qryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractSearchResponse
	for _, item := range result {

		var expDate *string
		if item.ExpDate != nil {
			s := item.ExpDate.Format(time.RFC3339)
			expDate = &s
		}

		var expPolicy *string
		if item.ExpPolicy != nil {
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &contractworkflowengine.ContractSearchResponse{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
		})
	}

	return contracts, nil
}

func (s *contractWorkflowEnginesrvc) Approve(ctx context.Context, req *contractworkflowengine.ContractApproveRequest) (res *contractworkflowengine.ContractApproveResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	updatedAt, _, err = s.runWorkflowGate(ctx, "approval", req.Did, updatedAt, map[string]any{
		"requested_by": middleware.GetParticipantID(ctx),
		"holder_did":   middleware.GetHolderDID(ctx),
		"user_roles":   workflowRoles(ctx),
		"causer_did":   localPeer,
	})
	if err != nil {
		return nil, err
	}

	cmd := command.ApproveCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		ApprovedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		CauserDID:  localPeer,
	}
	handler := command.Approver{
		DB:          s.DB,
		CRepo:       s.CRepo,
		ATRepo:      s.ATRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractApproveResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Reject(ctx context.Context, req *contractworkflowengine.ContractRejectRequest) (res *contractworkflowengine.ContractRejectResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.RejectCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		RejectedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		Reason:     req.Reason,
		CauserDID:  localPeer,
	}
	handler := command.Rejecter{
		DB:          s.DB,
		CRepo:       s.CRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractRejectResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Store(ctx context.Context, req *contractworkflowengine.ContractStoreRequest) (res *contractworkflowengine.ContractStoreResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.RecordEvidenceCmd{
		DID:        req.Did,
		RecordedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
		UpdatedAt:  updatedAt,
		CauserDID:  localPeer,
	}
	handler := command.EvidenceRecorder{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractStoreResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Terminate(ctx context.Context, req *contractworkflowengine.ContractTerminateRequest) (res *contractworkflowengine.ContractTerminateResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.TerminateCmd{
		DID:          req.Did,
		UpdatedAt:    updatedAt,
		TerminatedBy: middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		Reason:       req.Reason,
		CauserDID:    localPeer,
	}
	handler := command.Terminator{
		DB:          s.DB,
		CRepo:       s.CRepo,
		NRepo:       s.NRepo,
		NTRepo:      s.NTRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		SRepo:       s.SRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractTerminateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Renew(ctx context.Context, req *contractworkflowengine.ContractRenewRequest) (res *contractworkflowengine.ContractRenewResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	did, err := base.GenerateID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var newStartDate, newExpDate *time.Time
	if req.NewStartDate != nil {
		parsed, err := time.Parse(time.RFC3339, *req.NewStartDate)
		if err != nil {
			return nil, contractworkflowengine.MakeBadRequest(err)
		}
		newStartDate = &parsed
	}
	if req.NewExpDate != nil {
		parsed, err := time.Parse(time.RFC3339, *req.NewExpDate)
		if err != nil {
			return nil, contractworkflowengine.MakeBadRequest(err)
		}
		newExpDate = &parsed
	}

	cmd := command.RenewCmd{
		DID:                *did,
		OriginalDID:        req.Did,
		RenewedBy:          middleware.GetParticipantID(ctx),
		HolderDID:          middleware.GetHolderDID(ctx),
		UserRoles:          middleware.GetUserRoles(ctx),
		UpdatedAt:          updatedAt,
		NewStartDate:       newStartDate,
		NewExpDate:         newExpDate,
		NewExpPolicy:       req.NewExpPolicy,
		NewExpNoticePeriod: req.NewExpNoticePeriod,
	}
	handler := command.Renewer{
		DB:          s.DB,
		CRepo:       s.CRepo,
		RTRepo:      s.RTRepo,
		ATRepo:      s.ATRepo,
		NTRepo:      s.NTRepo,
		DIDDocument: s.DIDDocument,
	}
	result, err := handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractRenewResponse{
		Did:                   *did,
		RenewsDid:             req.Did,
		RenewsContractVersion: result.OriginalContractVersion,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Offer(ctx context.Context, req *contractworkflowengine.ContractOfferRequest) (res *contractworkflowengine.ContractOfferResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	updatedAt, _, err = s.runWorkflowGate(ctx, "offer", req.Did, updatedAt, map[string]any{
		"requested_by": middleware.GetParticipantID(ctx),
		"holder_did":   middleware.GetHolderDID(ctx),
		"user_roles":   workflowRoles(ctx),
		"causer_did":   localPeer,
	})
	if err != nil {
		return nil, err
	}

	cmd := command.OfferCmd{
		DID:       req.Did,
		UpdatedAt: updatedAt,
		OfferedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
		CauserDID: localPeer,
	}
	handler := command.Offerer{
		DB:          s.DB,
		CRepo:       s.CRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractOfferResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Withdraw(ctx context.Context, req *contractworkflowengine.ContractWithdrawRequest) (res *contractworkflowengine.ContractWithdrawResponse, err error) {

	err = s.DIDDocument.VerifyEIDASCertificate(s.TrustPool)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.WithdrawCmd{
		DID:         req.Did,
		UpdatedAt:   updatedAt,
		WithdrawnBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		CauserDID:   localPeer,
	}
	handler := command.Withdrawer{
		DB:          s.DB,
		CRepo:       s.CRepo,
		DIDDocument: s.DIDDocument,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractWithdrawResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Audit(ctx context.Context, req *contractworkflowengine.ContractAuditRequest) (res []*contractworkflowengine.ContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := qry2.GetAuditLogByDIDQry{
		DID:       req.Did,
		Scope:     componenttype.ContractWorkflowEngine,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	handler := qry2.AuditLogByDIDAuditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	auditLogHistory, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	history := make([]*contractworkflowengine.ContractAuditResponse, 0)
	for _, entry := range auditLogHistory {
		if !base.IsAuditVisibleEventType(entry.EventType) {
			continue
		}
		history = append(history, &contractworkflowengine.ContractAuditResponse{
			ID:            entry.ID,
			Component:     entry.Component,
			EventType:     entry.EventType,
			EventData:     entry.EventData,
			Did:           entry.DID,
			CreatedAt:     entry.CreatedAt.String(),
			ResLogPredCid: entry.ResLogPredCID,
		})
	}

	return history, nil
}

// retrieve templates
func (s *contractWorkflowEnginesrvc) RetrieveTemplates(ctx context.Context, req *contractworkflowengine.ApprovedContractTemplateRetrieveRequest) (res []*contractworkflowengine.ApprovedContractTemplateRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate2.GetAllApprovedTemplatesQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := contracttemplate2.GetAllApprovedTemplateHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*contractworkflowengine.ApprovedContractTemplateRetrieveResponse
	for _, item := range result {
		contractTemplates = append(contractTemplates, &contractworkflowengine.ApprovedContractTemplateRetrieveResponse{
			Did:          item.DID,
			Version:      item.Version,
			State:        item.State.String(),
			TemplateType: item.TemplateType.String(),
			Name:         item.Name,
			Description:  item.Description,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
			Responsible:  item.Responsible,
		})
	}

	return contractTemplates, nil
}

func (s *contractWorkflowEnginesrvc) Deploy(ctx context.Context, req *contractworkflowengine.ContractDeployRequest) (res *contractworkflowengine.ContractDeployResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	targetID := ""
	if req.TargetID != nil {
		targetID = *req.TargetID
	}
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	updatedAt, _, err = s.runWorkflowGate(ctx, "deployment", req.Did, updatedAt, map[string]any{
		"requested_by": middleware.GetParticipantID(ctx),
		"causer_did":   localPeer,
		"target_id":    targetID,
	})
	if err != nil {
		return nil, err
	}

	handler := command.Deployer{
		DB:             s.DB,
		CRepo:          s.CRepo,
		DeploymentRepo: s.DeploymentRepo,
		TargetRepo:     s.TargetRepo,
		Target:         s.TargetClient,
		PeerSigs:       s.SRepo,
	}
	result, err := handler.Handle(ctx, command.DeployCmd{
		DID:              req.Did,
		UpdatedAt:        updatedAt,
		RequestedBy:      middleware.GetParticipantID(ctx),
		LocalPeer:        localPeer,
		TargetIDOverride: targetID,
	})
	if err != nil {
		return nil, mapContractCommandError(err)
	}

	return &contractworkflowengine.ContractDeployResponse{
		Did:             result.DID,
		ContractVersion: result.ContractVersion,
		ContentHash:     result.ContentHash,
		Timestamp:       result.Timestamp.Format(time.RFC3339Nano),
		CorrelationID:   result.CorrelationID,
		Payload:         result.Payload,
		TargetID:        &result.TargetID,
		TargetName:      &result.TargetName,
	}, nil
}

func (s *contractWorkflowEnginesrvc) DeploymentCallback(ctx context.Context, req *contractworkflowengine.ContractDeploymentCallbackRequest) (res *contractworkflowengine.ContractDeploymentCallbackResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	// The target authenticates as itself: for a machine caller the validated
	// token's client_id is carried as the holder identity, and the handler only
	// accepts it for deployments dispatched to that target (ADR-27).
	cmd := command.DeploymentCallbackCmd{
		DID:            req.Did,
		CorrelationID:  req.CorrelationID,
		CallerClientID: middleware.GetHolderDID(ctx),
	}
	if req.Status != nil {
		cmd.Status = *req.Status
	}
	if req.Receipt != nil {
		receipt := &command.DeploymentReceiptPayload{}
		if req.Receipt.CorrelationID != nil {
			receipt.CorrelationID = *req.Receipt.CorrelationID
		}
		if req.Receipt.PayloadHash != nil {
			receipt.PayloadHash = *req.Receipt.PayloadHash
		}
		if req.Receipt.ActivatedAt != nil {
			receipt.ActivatedAt = *req.Receipt.ActivatedAt
		}
		cmd.Receipt = receipt
	}
	if req.Kpi != nil {
		if req.Kpi.Metric != nil {
			cmd.KPIMetric = *req.Kpi.Metric
		}
		if req.Kpi.Value != nil {
			cmd.KPIValue = *req.Kpi.Value
		}
		if req.Kpi.Verdict != nil {
			cmd.KPIVerdict = *req.Kpi.Verdict
		}
		if req.Kpi.Rule != nil {
			cmd.KPIRule = *req.Kpi.Rule
		}
	}

	handler := command.DeploymentCallbackHandler{
		DB:             s.DB,
		CRepo:          s.CRepo,
		DeploymentRepo: s.DeploymentRepo,
		TargetRepo:     s.TargetRepo,
		ArchiveTSA:     s.ArchiveTSA,
	}
	if err := handler.Handle(ctx, cmd); err != nil {
		switch {
		case errors.Is(err, command.ErrDeploymentCallbackUnauthorized):
			return nil, contractworkflowengine.MakeUnauthorized(err)
		default:
			return nil, mapContractCommandError(err)
		}
	}

	status := "OK"
	return &contractworkflowengine.ContractDeploymentCallbackResponse{
		Did:    req.Did,
		Status: &status,
	}, nil
}

// Resolve dereferences a contract's resource IRI: GET /contract/{did}
// serves the canonical JSON-LD contract document, under the same party
// read authorization retrieve_by_id enforces.
func (s *contractWorkflowEnginesrvc) Resolve(ctx context.Context, req *contractworkflowengine.ContractRetrieveByIDRequest) (any, error) {
	contract, err := s.RetrieveByID(ctx, req)
	if err != nil {
		return nil, err
	}
	return contract.ContractData, nil
}

// ---- Contract target registry (ADR-25) -------------------------------------

// contractTargetView maps a stored registry entry to its API shape.
func contractTargetView(target *db.ContractTarget) *contractworkflowengine.ContractTarget {
	view := &contractworkflowengine.ContractTarget{
		ID:        target.ID,
		Name:      target.Name,
		URL:       target.URL,
		Enabled:   target.Enabled,
		CreatedAt: ptr(target.CreatedAt.UTC().Format(time.RFC3339)),
		UpdatedAt: ptr(target.UpdatedAt.UTC().Format(time.RFC3339)),
	}
	if target.Description != nil {
		view.Description = target.Description
	}
	// Which client the target authenticates its callbacks as, and how old that
	// credential is. The secret itself is not here and is not stored: Hydra
	// keeps only a hash of it (ADR-27).
	if target.OAuthClientID != nil {
		view.OauthClientID = target.OAuthClientID
	}
	if target.SecretIssuedAt != nil {
		view.SecretIssuedAt = ptr(target.SecretIssuedAt.UTC().Format(time.RFC3339))
	}
	return view
}

func ptr[T any](v T) *T { return &v }

func (s *contractWorkflowEnginesrvc) ListContractTargets(ctx context.Context, req *contractworkflowengine.ContractTargetListRequest) (res []*contractworkflowengine.ContractTarget, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := s.TargetRepo.ListTargets(ctx, tx)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	res = make([]*contractworkflowengine.ContractTarget, 0, len(targets))
	for i := range targets {
		res = append(res, contractTargetView(&targets[i]))
	}
	return res, nil
}

func (s *contractWorkflowEnginesrvc) CreateContractTarget(ctx context.Context, req *contractworkflowengine.ContractTargetCreateRequest) (res *contractworkflowengine.ContractTarget, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("a target system needs a name and a URL"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	created, err := s.TargetRepo.CreateTarget(ctx, tx, db.ContractTarget{
		Name:        strings.TrimSpace(req.Name),
		URL:         strings.TrimSpace(req.URL),
		Description: req.Description,
		Enabled:     enabled,
		CreatedBy:   middleware.GetParticipantID(ctx),
	})
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	return contractTargetView(created), nil
}

func (s *contractWorkflowEnginesrvc) UpdateContractTarget(ctx context.Context, req *contractworkflowengine.ContractTargetUpdateRequest) (res *contractworkflowengine.ContractTarget, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("a target system needs a name and a URL"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := s.TargetRepo.UpdateTarget(ctx, tx, db.ContractTarget{
		ID:          req.ID,
		Name:        strings.TrimSpace(req.Name),
		URL:         strings.TrimSpace(req.URL),
		Description: req.Description,
		Enabled:     enabled,
	})
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}
	if updated == nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("no target system %s is registered", req.ID))
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	return contractTargetView(updated), nil
}

func (s *contractWorkflowEnginesrvc) DeleteContractTarget(ctx context.Context, req *contractworkflowengine.ContractTargetDeleteRequest) (err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	// Refused while a contract still names it: removing the entry would leave
	// those contracts undeployable with no record of where they were meant to
	// go. The admin repoints them first (ADR-25).
	designating, err := s.TargetRepo.CountContractsDesignating(ctx, tx, req.ID)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	if designating > 0 {
		return contractworkflowengine.MakeBadRequest(fmt.Errorf(
			"%d contract(s) still deploy to this target system; designate another one for them first", designating))
	}
	// The credential's authority goes with the target it belonged to. Its
	// registry row is what grants the Contract Target System scope, so leaving
	// it behind would keep a client authorised for a destination that no longer
	// exists. The Hydra client itself is left alone: a target whose client comes
	// from deployment configuration shares it with a declared system client, and
	// removing that from here would take a configured credential away with no
	// way to put it back short of a reinstall. Without a registry row it
	// resolves to no caller and is refused everywhere.
	target, err := s.TargetRepo.ReadTarget(ctx, tx, req.ID)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	if err := s.TargetRepo.DeleteTarget(ctx, tx, req.ID); err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	if target != nil && target.OAuthClientID != nil && strings.TrimSpace(*target.OAuthClientID) != "" {
		if err := s.MachineIdentities.DeleteByClientIDTx(ctx, tx, strings.TrimSpace(*target.OAuthClientID)); err != nil {
			return contractworkflowengine.MakeInternalError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	return nil
}

func (s *contractWorkflowEnginesrvc) DesignateContractTarget(ctx context.Context, req *contractworkflowengine.ContractTargetDesignateRequest) (res *contractworkflowengine.ContractRetrieveByIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	// An empty target_id clears the designation; anything else must name a
	// registered, enabled entry. Designating a disabled target would produce a
	// contract that deploys nowhere and only says so at signing time.
	var targetID *string
	if req.TargetID != nil && strings.TrimSpace(*req.TargetID) != "" {
		id := strings.TrimSpace(*req.TargetID)
		target, err := s.TargetRepo.ReadTarget(ctx, tx, id)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		if target == nil {
			return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("no target system %s is registered", id))
		}
		if !target.Enabled {
			return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("target system %q is disabled and cannot receive deployments", target.Name))
		}
		targetID = &id
	}

	// Staleness is compared at second granularity against the stored value,
	// the way every other contract mutation does it: updated_at is handed to
	// clients as RFC3339, which carries no sub-second part, so an exact match
	// could never succeed.
	stored, err := s.CRepo.ReadDataByDID(ctx, tx, req.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if stored == nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("no contract %s", req.Did))
	}
	if updatedAt.Unix() < stored.UpdatedAt.Unix() {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("the contract changed since it was read; reload and try again"))
	}

	changed, err := s.TargetRepo.DesignateForContract(ctx, tx, req.Did, targetID)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if !changed {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("no contract %s", req.Did))
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return s.RetrieveByID(ctx, &contractworkflowengine.ContractRetrieveByIDRequest{Did: req.Did, Token: req.Token})
}
