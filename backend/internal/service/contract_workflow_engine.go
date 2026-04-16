package service

import (
	"context"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationactionflag"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	contracttemplatequery "digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"
)

type contractWorkflowEnginesrvc struct {
	DB       *sqlx.DB
	CRepo    db.ContractRepo
	RTRepo   db.ReviewTaskRepo
	ATRepo   db.ApprovalTaskRepo
	NTRepo   db.NegotiationTaskRepo
	NRepo    db.NegotiationRepo
	CTRepo   db.ContractTemplateRepo
	FCClient *fcclient.FederatedCatalogueClient
	auth.JWTAuthenticator
}

func NewContractWorkflowEngine(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, fcClient *fcclient.FederatedCatalogueClient) contractworkflowengine.Service {

	return &contractWorkflowEnginesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		CTRepo:           ctRepo,
		FCClient:         fcClient,
	}
}

func (s *contractWorkflowEnginesrvc) Create(ctx context.Context, req *contractworkflowengine.ContractCreateRequest) (res *contractworkflowengine.ContractCreateResponse, err error) {

	did, err := base.GetDID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	queryHandler := contracttemplatequery.GetTemplateDataByDIDHandler{
		Ctx:      ctx,
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		FCClient: s.FCClient,
	}
	contractData, err := queryHandler.Handle(contracttemplatequery.GetTemplateDataByDIDQry{
		Token: *req.Token,
		DID:   req.Did,
	})
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.CreateCmd{
		DID:          *did,
		TemplateDID:  req.Did,
		CreatedBy:    middleware.GetUsername(ctx),
		ContractData: contractData,
	}
	createHandler := command.Creator{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = createHandler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractCreateResponse{
		Did: *did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Update(ctx context.Context, req *contractworkflowengine.ContractUpdateRequest) (res *contractworkflowengine.ContractUpdateResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	metaData, err := datatype.NewJSON(req.ContractData)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.UpdateCmd{
		DID:             req.Did,
		ContractVersion: req.ContractVersion,
		UpdatedAt:       updatedAt,
		UpdatedBy:       middleware.GetUsername(ctx),
		Name:            req.Name,
		Description:     req.Description,
		ContractData:    &metaData,
	}
	handler := command.Updater{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractUpdateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Submit(ctx context.Context, req *contractworkflowengine.ContractSubmitRequest) (res *contractworkflowengine.ContractSubmitResponse, err error) {

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

	cmd := command.SubmitCmd{
		DID:         req.Did,
		UpdatedAt:   updatedAt,
		SubmittedBy: middleware.GetUsername(ctx),
		ActionFlag:  actionFlag,
		Comments:    req.Comments,
		Reviewers:   req.Reviewers,
		Approver:    req.Approver,
		Negotiators: req.Negotiators,
	}
	handler := command.Submitter{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NRepo:  s.NRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractSubmitResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Retrieve(ctx context.Context, req *contractworkflowengine.ContractRetrieveRequest) (res *contractworkflowengine.ContractRetrieveResponse, err error) {

	qry := contract.GetAllMetadataQry{
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contract.GetAllMetadataHandler{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
		NTRepo: s.NTRepo,
	}
	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractItem
	for _, item := range result.Contracts {
		contracts = append(contracts, &contractworkflowengine.ContractItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
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

	qry := contract.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
		NRepo: s.NRepo,
	}
	contractResult, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	negotiations := make(map[string]*contractworkflowengine.ContractNegotiationItem)
	for _, item := range contractResult.Negotiations {
		negotiation, ok := negotiations[item.ID]
		if !ok {
			negotiation = &contractworkflowengine.ContractNegotiationItem{
				ID:            item.ID,
				ChangeRequest: item.ChangeRequest,
				CreatedBy:     item.CreatedBy,
				CreatedAt:     item.CreatedAt.String(),
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

	return &contractworkflowengine.ContractRetrieveByIDResponse{
		Did:             contractResult.DID,
		ContractVersion: contractResult.ContractVersion,
		State:           contractResult.State.String(),
		Name:            contractResult.Name,
		Description:     contractResult.Description,
		CreatedBy:       contractResult.CreatedBy,
		CreatedAt:       contractResult.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       contractResult.UpdatedAt.Format(time.RFC3339),
		ContractData:    contractResult.ContractData,
		Negotiations:    negotiationList,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Negotiate(ctx context.Context, req *contractworkflowengine.ContractNegotiationRequest) (res *contractworkflowengine.ContractNegotiationResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	changeRequest, err := datatype.NewJSON(req.ChangeRequest)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.NegotiationCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		NegotiatedBy:  middleware.GetUsername(ctx),
		ChangeRequest: &changeRequest,
	}
	handler := command.Negotiator{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		NRepo:  s.NRepo,
		RTRepo: s.RTRepo,
		NTRepo: s.NTRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractNegotiationResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Respond(ctx context.Context, req *contractworkflowengine.ContractNegotiationRespondRequest) (res *contractworkflowengine.ContractNegotiationRespondResponse, err error) {

	actionFlag, err := negotiationactionflag.NewNegotiationActionFlag(req.ActionFlag)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(fmt.Errorf("unknown action flag: %s", req.ActionFlag))
	}

	if actionFlag == negotiationactionflag.Accepting {

		cmd := command.AcceptNegotiationCmd{
			ID:         req.ID,
			AcceptedBy: middleware.GetUsername(ctx),
		}
		handler := command.NegotiationAcceptor{
			Ctx:    ctx,
			DB:     s.DB,
			CRepo:  s.CRepo,
			NRepo:  s.NRepo,
			NTRepo: s.NTRepo,
		}
		err = handler.Handle(cmd)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

	} else if actionFlag == negotiationactionflag.Rejecting {

		cmd := command.RejectNegotiationCmd{
			ID:              req.ID,
			RejectedBy:      middleware.GetUsername(ctx),
			RejectionReason: req.RejectionReason,
		}
		handler := command.NegotiationRejector{
			Ctx:    ctx,
			DB:     s.DB,
			CRepo:  s.CRepo,
			NRepo:  s.NRepo,
			NTRepo: s.NTRepo,
		}
		err = handler.Handle(cmd)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	}

	return &contractworkflowengine.ContractNegotiationRespondResponse{
		ID: req.ID,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Review(ctx context.Context, req *contractworkflowengine.ContractReviewRequest) (res *contractworkflowengine.ContractReviewResponse, err error) {

	cmd := contract.ReviewCmd{
		DID:        req.Did,
		ReviewedBy: middleware.GetUsername(ctx),
	}
	handler := contract.Reviewer{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractReviewResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Search(ctx context.Context, req *contractworkflowengine.ContractSearchRequest) (res []*contractworkflowengine.ContractSearchResponse, err error) {

	var state *contractstate.ContractState
	if req.State != nil {
		tState, err := contractstate.NewContractState(*req.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}

		state = &tState
	}

	qry := contract.GetAllMetadataByFilterQry{
		DID:             req.Did,
		ContractVersion: req.ContractVersion,
		State:           state,
		RetrievedBy:     middleware.GetUsername(ctx),
		Name:            req.Name,
		Description:     req.Description,
		Filter:          req.Filter,
	}
	queryHandler := contract.GetAllMetaDataByFilterHandler{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractworkflowengine.ContractSearchResponse
	for _, item := range result {
		contracts = append(contracts, &contractworkflowengine.ContractSearchResponse{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		})
	}

	return contracts, nil
}

func (s *contractWorkflowEnginesrvc) Approve(ctx context.Context, req *contractworkflowengine.ContractApproveRequest) (res *contractworkflowengine.ContractApproveResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.ApproveCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		ApprovedBy: middleware.GetUsername(ctx),
	}
	handler := command.Approver{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractApproveResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Reject(ctx context.Context, req *contractworkflowengine.ContractRejectRequest) (res *contractworkflowengine.ContractRejectResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.RejectCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		RejectedBy: middleware.GetUsername(ctx),
		Reason:     req.Reason,
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractRejectResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Store(ctx context.Context, req *contractworkflowengine.ContractStoreRequest) (res *contractworkflowengine.ContractStoreResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := command.RecordEvidenceCmd{
		DID:        req.Did,
		RecordedBy: middleware.GetUsername(ctx),
		UpdatedAt:  updatedAt,
	}
	handler := command.EvidenceRecorder{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractStoreResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Terminate(ctx context.Context, req *contractworkflowengine.ContractTerminateRequest) (res *contractworkflowengine.ContractTerminateResponse, err error) {

	cmd := command.TerminateCmd{
		DID:          req.Did,
		TerminatedBy: middleware.GetUsername(ctx),
		Reason:       req.Reason,
	}
	handler := command.Terminator{
		Ctx:    ctx,
		DB:     s.DB,
		CRepo:  s.CRepo,
		NRepo:  s.NRepo,
		NTRepo: s.NTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractTerminateResponse{
		Did: req.Did,
	}, nil
}

func (s *contractWorkflowEnginesrvc) Audit(ctx context.Context, req *contractworkflowengine.ContractAuditRequest) (res *contractworkflowengine.ContractAuditResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	cmd := contract.AuditCmd{
		DID:       req.Did,
		AuditedBy: middleware.GetUsername(ctx),
		UpdatedAt: updatedAt,
	}
	handler := contract.Auditor{
		Ctx:   ctx,
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &contractworkflowengine.ContractAuditResponse{
		Did: req.Did,
	}, nil
}
