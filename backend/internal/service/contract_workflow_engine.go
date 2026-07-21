package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"digital-contracting-service/internal/dcstodcs"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/semantichub"

	contracttemplate2 "digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
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
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"

	"github.com/jmoiron/sqlx"
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
	FCClient             *fcclient.FederatedCatalogueClient
	DIDDocument          identity.DIDDocument
	ATrailReader         base.AuditTrailReader
	DCSToDCSSynchronizer dcstodcs.DCSToDCSSynchronizer
	TrustPool            *identity.EUTrustPool
	IPFSClient           *ipfs.APIClient
	ArchiveNotary        command.ArchiveNotary
	ArchiveTSA           *tsa.APIClient
	TargetClient         command.ContractTargetClient
	auth.JWTAuthenticator
}

func NewContractWorkflowEngine(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo,
	sRepo db2.SyncRepository, trustPool *identity.EUTrustPool,
	fcClient *fcclient.FederatedCatalogueClient, auditTrailReader base.AuditTrailReader, didDocument identity.DIDDocument,
	ipfsClient *ipfs.APIClient, archiveNotary command.ArchiveNotary, archiveTSA *tsa.APIClient,
	deploymentRepo db.DeploymentRepo, targetClient command.ContractTargetClient) contractworkflowengine.Service {

	return &contractWorkflowEnginesrvc{
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
		FCClient:         fcClient,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
		TrustPool:        trustPool,
		IPFSClient:       ipfsClient,
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
		TargetClient:     targetClient,
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
	if errors.Is(err, contractstate.ErrInvalidTransition) ||
		errors.Is(err, validation.ErrContractHierarchyInvalid) ||
		errors.Is(err, validation.ErrContractNotClosed) ||
		errors.Is(err, command.ErrContractHierarchyCycle) ||
		errors.Is(err, command.ErrDeploymentNotFound) ||
		errors.Is(err, command.ErrSigningIncomplete) ||
		errors.Is(err, command.ErrContractNotRenewable) ||
		errors.Is(err, command.ErrNotAParty) ||
		errors.Is(err, command.ErrConflictOfInterest) ||
		errors.Is(err, db.ErrNoMatchingDecision) {
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

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	counterparty := ""
	if req.Counterparty != nil {
		counterparty = *req.Counterparty
	}
	if counterparty != "" {
		untrustedPeers, err := dcstodcs.CheckForUntrustedPeers(ctx, s.DB, s.SRepo, localPeer, []string{counterparty})
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		if len(untrustedPeers) > 0 {
			return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("untrusted counterparty is not allowed: %v", untrustedPeers))
		}
	}

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
		return nil, contractworkflowengine.MakeInternalError(err)
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
			negotiation = &contractworkflowengine.ContractNegotiationItem{
				ID:              item.ID,
				ContractVersion: item.ContractVersion,
				ChangeRequest:   item.ChangeRequest,
				CreatedBy:       item.CreatedBy,
				CreatedAt:       item.CreatedAt.String(),
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

	kpis, kpiViolations, err := s.retrieveKPIs(ctx, req.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	extrinsic := string(contractstate.InferExtrinsic(contractResult.State.String()))
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
		KpiViolations:      kpiViolations,
	}, nil
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
		observations = append(observations, map[string]any{
			"@id":               fmt.Sprintf("%s#kpi-%d", base.ResourceIRI("contract", req.Did), entry.ID),
			"@type":             "dcs:KPIObservation",
			"dcs:metricName":    entry.Metric,
			"dcs:observedValue": entry.Value,
			"dcs:observedAt":    entry.ObservedAt.Format(time.RFC3339),
			"dcs:violation":     entry.Violation,
			"dcs:aboutContract": map[string]any{"@id": base.ResourceIRI("contract", req.Did)},
		})
	}
	return map[string]any{
		"@context":        semantichub.AnchorURL("context", semantichub.ContextName, contextVersion),
		"@id":             req.Did + "#kpi-observations",
		"@type":           "dcs:KPIObservationSet",
		"dcs:observation": observations,
	}, nil
}

// retrieveKPIs reads the KPI values reported via deployment callbacks for a
// contract (DCS-FR-CWE-31, DCS-FR-CWE-09), returning both the per-KPI list
// and the distinct set of metric names whose latest reported value violates
// its contractual SLA threshold.
func (s *contractWorkflowEnginesrvc) retrieveKPIs(ctx context.Context, did string) ([]*contractworkflowengine.ContractDeploymentKPIItem, []string, error) {
	if s.DeploymentRepo == nil {
		return nil, nil, nil
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := s.DeploymentRepo.ReadKPIsByDID(ctx, tx, did)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read KPIs for contract %s: %w", did, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	kpis := make([]*contractworkflowengine.ContractDeploymentKPIItem, 0, len(entries))
	latestViolation := map[string]bool{}
	order := make([]string, 0)
	for _, entry := range entries {
		violation := entry.Violation
		kpis = append(kpis, &contractworkflowengine.ContractDeploymentKPIItem{
			Metric:     entry.Metric,
			Value:      entry.Value,
			ObservedAt: entry.ObservedAt.Format(time.RFC3339),
			Violation:  &violation,
		})
		if _, seen := latestViolation[entry.Metric]; !seen {
			order = append(order, entry.Metric)
		}
		latestViolation[entry.Metric] = violation
	}

	violations := make([]string, 0)
	for _, metric := range order {
		if latestViolation[metric] {
			violations = append(violations, metric)
		}
	}

	return kpis, violations, nil
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
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			TemplateType:   item.TemplateType.String(),
			Name:           item.Name,
			Description:    item.Description,
			CreatedBy:      item.CreatedBy,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
			Responsible:    item.Responsible,
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

	handler := command.Deployer{
		DB:             s.DB,
		CRepo:          s.CRepo,
		DeploymentRepo: s.DeploymentRepo,
		Target:         s.TargetClient,
	}
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	result, err := handler.Handle(ctx, command.DeployCmd{
		DID:         req.Did,
		UpdatedAt:   updatedAt,
		RequestedBy: middleware.GetParticipantID(ctx),
		LocalPeer:   localPeer,
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
	}, nil
}

func (s *contractWorkflowEnginesrvc) DeploymentCallback(ctx context.Context, req *contractworkflowengine.ContractDeploymentCallbackRequest) (res *contractworkflowengine.ContractDeploymentCallbackResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	cmd := command.DeploymentCallbackCmd{
		DID:           req.Did,
		CorrelationID: req.CorrelationID,
	}
	if req.CallbackSecret != nil {
		cmd.Secret = *req.CallbackSecret
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
	}

	handler := command.DeploymentCallbackHandler{
		DB:             s.DB,
		CRepo:          s.CRepo,
		DeploymentRepo: s.DeploymentRepo,
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
