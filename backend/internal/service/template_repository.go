package service

import (
	"context"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"time"

	"github.com/jmoiron/sqlx"
)

// TemplateRepository service example implementation.
// The example methods log the requests and return zero values.
type templateRepositorysrvc struct {
	DB           *sqlx.DB
	CTRepo       db.ContractTemplateRepo
	RTRepo       db.ReviewTaskRepo
	ATRepo       db.ApprovalTaskRepo
	FCClient     *fcclient.FederatedCatalogueClient
	ATrailReader base.AuditTrailReader
	auth.JWTAuthenticator
}

// NewTemplateRepository returns the TemplateRepository service implementation.
func NewTemplateRepository(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, CTRepo db.ContractTemplateRepo,
	RTRepo db.ReviewTaskRepo, ATRepo db.ApprovalTaskRepo, fcClient *fcclient.FederatedCatalogueClient, auditTrailReader base.AuditTrailReader) templaterepository.Service {
	return &templateRepositorysrvc{
		DB:               db,
		JWTAuthenticator: jwtAuth,
		CTRepo:           CTRepo,
		RTRepo:           RTRepo,
		ATRepo:           ATRepo,
		FCClient:         fcClient,
		ATrailReader:     auditTrailReader,
	}
}

// Create a new template.
func (s *templateRepositorysrvc) Create(ctx context.Context, req *templaterepository.ContractTemplateCreateRequest) (*templaterepository.ContractTemplateCreateResponse, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	templateType, err := contracttemplatetype.NewContractTemplateType(req.TemplateType)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	templateData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	did, err := base.GetDID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    middleware.GetUsername(ctx),
		TemplateType: templateType,
		Name:         req.Name,
		Description:  req.Description,
		TemplateData: &templateData,
	}
	createHandler := command.Creator{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateCreateResponse{
		Did: *did,
	}, nil
}

// Copy a new template.
func (s *templateRepositorysrvc) Copy(ctx context.Context, req *templaterepository.ContractTemplateCopyRequest) (*templaterepository.ContractTemplateCopyResponse, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	did, err := base.GetDID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.CopyCmd{
		NewDID:   *did,
		CopyDID:  req.Did,
		CopiedBy: middleware.GetUsername(ctx),
	}
	copyHandler := command.Copier{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = copyHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateCopyResponse{
		Did: *did,
	}, nil
}

// with action flag { forwardTo: "approval" | "draft" } and optional
// reviewComments. allow resubmission path with approver comments.
func (s *templateRepositorysrvc) Submit(ctx context.Context, req *templaterepository.ContractTemplateSubmitRequest) (res *templaterepository.ContractTemplateSubmitResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var actionFlag *actionflag.ActionFlag
	if req.ForwardTo != nil {
		flag, err := actionflag.NewActionFlag(*req.ForwardTo)
		if err != nil {
			return nil, templaterepository.MakeInternalError(err)
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
	}
	handler := command.Submitter{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateSubmitResponse{
		Did: req.Did,
	}, nil
}

// persist reviewer edits (metadata/clauses/semantics).
func (s *templateRepositorysrvc) Update(ctx context.Context, req *templaterepository.ContractTemplateUpdateRequest) (res *templaterepository.ContractTemplateUpdateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	metaData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var templateType *contracttemplatetype.ContractTemplateType
	if req.TemplateType != nil {
		tType, err := contracttemplatetype.NewContractTemplateType(*req.TemplateType)
		if err != nil {
			return nil, templaterepository.MakeInternalError(err)
		}
		templateType = &tType
	}

	cmd := command.UpdateCmd{
		DID:            req.Did,
		DocumentNumber: req.DocumentNumber,
		UpdatedAt:      updatedAt,
		TemplateType:   templateType,
		Name:           req.Name,
		Description:    req.Description,
		TemplateData:   &metaData,
		UpdatedBy:      middleware.GetUsername(ctx),
	}
	handler := command.Updater{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateUpdateResponse{
		Did: req.Did,
	}, nil
}

// update metadata or status.
func (s *templateRepositorysrvc) UpdateManage(ctx context.Context, req *templaterepository.ContractTemplateUpdateManageRequest) (res *templaterepository.ContractTemplateUpdateManageResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	metaData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var state *contracttemplatestate.ContractTemplateState
	if req.State != nil {
		ts, err := contracttemplatestate.NewContractTemplateState(*req.State)
		if err != nil {
			return nil, templaterepository.MakeInternalError(err)
		}
		state = &ts
	}

	var templateType *contracttemplatetype.ContractTemplateType
	if req.TemplateType != nil {
		tType, err := contracttemplatetype.NewContractTemplateType(*req.TemplateType)
		if err != nil {
			return nil, templaterepository.MakeInternalError(err)
		}
		templateType = &tType
	}

	cmd := command.UpdateManageCmd{
		DID:            req.Did,
		DocumentNumber: req.DocumentNumber,
		State:          state,
		UpdatedAt:      updatedAt,
		TemplateType:   templateType,
		Name:           req.Name,
		Description:    req.Description,
		TemplateData:   &metaData,
		UpdatedBy:      middleware.GetUsername(ctx),
	}
	handler := command.UpdateManager{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateUpdateManageResponse{
		Did: req.Did,
	}, nil
}

// perform filtered searches.
func (s *templateRepositorysrvc) Search(ctx context.Context, req *templaterepository.ContractTemplateSearchRequest) (res []*templaterepository.ContractTemplateSearchResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contracttemplatestate.ContractTemplateState
	if req.State != nil {
		tState, err := contracttemplatestate.NewContractTemplateState(*req.State)
		if err != nil {
			return nil, templaterepository.MakeInternalError(err)
		}

		state = &tState
	}

	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy:    middleware.GetUsername(ctx),
		DID:            derefString(req.Did),
		DocumentNumber: derefString(req.DocumentNumber),
		Version:        derefInt(req.Version),
		State:          state,
		Name:           derefString(req.Name),
		Description:    derefString(req.Description),
		TemplateData:   derefString(req.TemplateData),
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateSearchResponse
	for _, item := range result {
		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateSearchResponse{
			Did:                item.DID,
			DocumentNumber:     item.DocumentNumber,
			Version:            item.Version,
			State:              item.State.String(),
			TemplateType:       item.TemplateType.String(),
			Name:               item.Name,
			Description:        item.Description,
			CreatedAt:          item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          item.UpdatedAt.Format(time.RFC3339),
			ResponsiblePersons: item.ResponsiblePersons,
		})
	}

	return contractTemplates, nil
}

func (s *templateRepositorysrvc) RetrieveHistoryByID(ctx context.Context, req *templaterepository.ContractTemplateHistoryRetrieveByIDRequest) (res []*templaterepository.ContractTemplateHistoryRetrieveByIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate.GetHistoryByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contracttemplate.GetHistoryByIDHandler{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateHistoryRetrieveByIDResponse
	for _, item := range result {

		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateHistoryRetrieveByIDResponse{
			Did:                item.DID,
			DocumentNumber:     item.DocumentNumber,
			Version:            item.Version,
			State:              item.State.String(),
			Name:               item.Name,
			Description:        item.Description,
			CreatedBy:          item.CreatedBy,
			CreatedAt:          item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          item.UpdatedAt.Format(time.RFC3339),
			ResponsiblePersons: item.ResponsiblePersons,
			TemplateData:       item.TemplateData,
			TemplateType:       item.TemplateType.String(),
		})
	}

	return contractTemplates, nil
}

// retrieve templates
func (s *templateRepositorysrvc) Retrieve(ctx context.Context, req *templaterepository.ContractTemplateRetrieveRequest) (res *templaterepository.ContractTemplateRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate.GetAllMetadataQry{
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contracttemplate.GetAllMetadataHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateItem
	for _, item := range result.ContractTemplates {
		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateItem{
			Did:                item.DID,
			DocumentNumber:     item.DocumentNumber,
			Version:            item.Version,
			State:              item.State.String(),
			TemplateType:       item.TemplateType.String(),
			Name:               item.Name,
			Description:        item.Description,
			CreatedBy:          item.CreatedBy,
			CreatedAt:          item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          item.UpdatedAt.Format(time.RFC3339),
			ResponsiblePersons: item.ResponsiblePersons,
		})
	}

	var reviewTasks []*templaterepository.ReviewTaskItem
	for _, item := range result.ReviewerTasks {
		reviewTasks = append(reviewTasks, &templaterepository.ReviewTaskItem{
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			Reviewer:       item.Reviewer,
			State:          item.State.String(),
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
		})
	}

	var approvalTasks []*templaterepository.ApprovalTaskItem
	for _, item := range result.ApprovalTasks {
		approvalTasks = append(approvalTasks, &templaterepository.ApprovalTaskItem{
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			Approver:       item.Approver,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
		})
	}

	return &templaterepository.ContractTemplateRetrieveResponse{
		ContractTemplates: contractTemplates,
		ReviewTasks:       reviewTasks,
		ApprovalTasks:     approvalTasks,
	}, nil
}

// Retrieve a template by template id.
func (s *templateRepositorysrvc) RetrieveByID(ctx context.Context, req *templaterepository.ContractTemplateRetrieveByIDRequest) (res *templaterepository.ContractTemplateRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRetrieveByIDResponse{
		Did:                contractTemplate.DID,
		DocumentNumber:     contractTemplate.DocumentNumber,
		Version:            contractTemplate.Version,
		State:              contractTemplate.State.String(),
		TemplateType:       contractTemplate.TemplateType.String(),
		Name:               contractTemplate.Name,
		Description:        contractTemplate.Description,
		CreatedBy:          contractTemplate.CreatedBy,
		CreatedAt:          contractTemplate.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          contractTemplate.UpdatedAt.Format(time.RFC3339),
		TemplateData:       contractTemplate.TemplateData,
		ResponsiblePersons: contractTemplate.ResponsiblePersons,
	}, nil
}

// run policy, schema, and semantic validations; return findings.
func (s *templateRepositorysrvc) Verify(ctx context.Context, req *templaterepository.ContractTemplateVerifyRequest) (res *templaterepository.ContractTemplateVerifyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	cmd := command.VerifyCmd{
		DID:           req.Did,
		VerifiedBy:    middleware.GetUsername(ctx),
		ParticipantID: middleware.GetParticipantID(ctx),
	}
	handler := command.Verifier{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		RTRepo:   s.RTRepo,
		FCClient: s.FCClient,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateVerifyResponse{
		Did: req.Did,
	}, nil
}

// mark template as approved, with optional decision notes.
func (s *templateRepositorysrvc) Approve(ctx context.Context, req *templaterepository.ContractTemplateApproveRequest) (res *templaterepository.ContractTemplateApproveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.ApproveCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		ApprovedBy:    middleware.GetUsername(ctx),
		DecisionNotes: req.DecisionNotes,
	}
	handler := command.Approver{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateApproveResponse{
		Did: req.Did,
	}, nil
}

// mark template as rejected, requiring reason field.
func (s *templateRepositorysrvc) Reject(ctx context.Context, req *templaterepository.ContractTemplateRejectRequest) (res *templaterepository.ContractTemplateRejectResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

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
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRejectResponse{
		Did: req.Did,
	}, nil
}

// register new template into the repository.
func (s *templateRepositorysrvc) Register(ctx context.Context, req *templaterepository.ContractTemplateRegisterRequest) (res *templaterepository.ContractTemplateRegisterResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	newDID, err := base.GetDID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.RegisterCmd{
		DID:          req.Did,
		NewDID:       *newDID,
		Version:      req.Version,
		RegisteredBy: middleware.GetUsername(ctx),
	}
	handler := command.Registrar{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		FCClient: s.FCClient,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRegisterResponse{
		Did: *newDID,
	}, nil
}

// archive obsolete template.
func (s *templateRepositorysrvc) Archive(ctx context.Context, req *templaterepository.ContractTemplateArchiveRequest) (res *templaterepository.ContractTemplateArchiveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.ArchiveCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		ArchivedBy: middleware.GetUsername(ctx),
	}
	handler := command.Archiver{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateArchiveResponse{
		Did: req.Did,
	}, nil
}

// retrieve audit history of template actions.
func (s *templateRepositorysrvc) Audit(ctx context.Context, req *templaterepository.ContractTemplateAuditRequest) (res []*templaterepository.ContractTemplateAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate.GetAuditLogQry{
		DID:       req.Did,
		AuditedBy: middleware.GetUsername(ctx),
	}
	handler := contracttemplate.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	auditLogHistory, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	history := make([]*templaterepository.ContractTemplateAuditResponse, 0)
	for _, entry := range auditLogHistory {
		history = append(history, &templaterepository.ContractTemplateAuditResponse{
			ID:               entry.ID,
			Component:        entry.Component,
			EventType:        entry.EventType,
			EventData:        entry.EventData,
			Did:              entry.DID,
			CreatedAt:        entry.CreatedAt.String(),
			GlobalLogPredCid: entry.GlobalLogPredCID,
			ResLogPredCid:    entry.ResLogPredCID,
		})
	}

	return history, nil
}

// publish approved template to Federated Catalogue.
func (s *templateRepositorysrvc) Publish(ctx context.Context, req *templaterepository.ContractTemplatePublishRequest) (res *templaterepository.ContractTemplatePublishResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.PublishCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		PublishedBy:   middleware.GetUsername(ctx),
		ParticipantID: middleware.GetParticipantID(ctx),
	}
	handler := command.Publisher{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		FCClient: s.FCClient,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplatePublishResponse{
		Did: req.Did,
	}, nil
}

func derefInt(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}
