package service

import (
	"context"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/middleware"
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
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	auth.JWTAuthenticator
}

// NewTemplateRepository returns the TemplateRepository service implementation.
func NewTemplateRepository(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, CTRepo db.ContractTemplateRepo,
	RTRepo db.ReviewTaskRepo, ATRepo db.ApprovalTaskRepo) templaterepository.Service {
	return &templateRepositorysrvc{
		DB:               db,
		JWTAuthenticator: jwtAuth,
		CTRepo:           CTRepo,
		RTRepo:           RTRepo,
		ATRepo:           ATRepo,
	}
}

// Create a new template.
func (s *templateRepositorysrvc) Create(ctx context.Context, req *templaterepository.ContractTemplateCreateRequest) (*templaterepository.ContractTemplateCreateResponse, error) {

	templateType, err := contracttemplatetype.NewContractTemplateType(req.TemplateType)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	jsonMetaData, err := datatype.NewJSON(req.TemplateData)
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
		TemplateData: &jsonMetaData,
	}
	createHandler := command.Creator{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = createHandler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateCreateResponse{
		Did: *did,
	}, nil
}

// with action flag { forwardTo: "approval" | "draft" } and optional
// reviewComments. allow resubmission path with approver comments.
func (s *templateRepositorysrvc) Submit(ctx context.Context, req *templaterepository.ContractTemplateSubmitRequest) (res *templaterepository.ContractTemplateSubmitResponse, err error) {

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
		Reviewer:    req.Reviewers,
		Approver:    req.Approver,
	}
	handler := command.Submitter{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateSubmitResponse{
		Did: req.Did,
	}, nil
}

// persist reviewer edits (metadata/clauses/semantics).
func (s *templateRepositorysrvc) Update(ctx context.Context, req *templaterepository.ContractTemplateUpdateRequest) (res *templaterepository.ContractTemplateUpdateResponse, err error) {

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
		Version:        req.Version,
		UpdatedAt:      updatedAt,
		TemplateType:   templateType,
		Name:           req.Name,
		Description:    req.Description,
		TemplateData:   &metaData,
		UpdatedBy:      middleware.GetUsername(ctx),
	}
	handler := command.Updater{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateUpdateResponse{
		Did: req.Did,
	}, nil
}

// update metadata or status.
func (s *templateRepositorysrvc) UpdateManage(ctx context.Context, req *templaterepository.ContractTemplateUpdateManageRequest) (res *templaterepository.ContractTemplateUpdateManageResponse, err error) {

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
		Version:        req.Version,
		State:          state,
		UpdatedAt:      updatedAt,
		TemplateType:   templateType,
		Name:           req.Name,
		Description:    req.Description,
		TemplateData:   &metaData,
		UpdatedBy:      middleware.GetUsername(ctx),
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateUpdateManageResponse{
		Did:            req.Did,
		DocumentNumber: req.DocumentNumber,
		Version:        req.Version,
	}, nil
}

// perform filtered searches.
func (s *templateRepositorysrvc) Search(ctx context.Context, req *templaterepository.ContractTemplateSearchRequest) (res []*templaterepository.ContractTemplateSearchResponse, err error) {

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
		DID:            req.Did,
		DocumentNumber: req.DocumentNumber,
		Version:        req.Version,
		State:          state,
		Name:           req.Name,
		Description:    req.Description,
		Filter:         req.Filter,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateSearchResponse
	for _, item := range result {
		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateSearchResponse{
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			TemplateType:   item.TemplateType.String(),
			Name:           item.Name,
			Description:    item.Description,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
		})
	}

	return contractTemplates, nil
}

// retrieve templates
func (s *templateRepositorysrvc) Retrieve(ctx context.Context, req *templaterepository.ContractTemplateRetrieveRequest) (res *templaterepository.ContractTemplateRetrieveResponse, err error) {

	qry := contracttemplate.GetAllMetadataQry{
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contracttemplate.GetAllMetadataHandler{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateItem
	for _, item := range result.ContractTemplates {
		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateItem{
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			TemplateType:   item.TemplateType.String(),
			Name:           item.Name,
			Description:    item.Description,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
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

	qry := contracttemplate.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetUsername(ctx),
	}
	queryHandler := contracttemplate.GetByIDHandler{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRetrieveByIDResponse{
		Did:            contractTemplate.DID,
		DocumentNumber: contractTemplate.DocumentNumber,
		Version:        contractTemplate.Version,
		State:          contractTemplate.State.String(),
		TemplateType:   contractTemplate.TemplateType.String(),
		Name:           contractTemplate.Name,
		Description:    contractTemplate.Description,
		CreatedBy:      contractTemplate.CreatedBy,
		CreatedAt:      contractTemplate.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      contractTemplate.UpdatedAt.Format(time.RFC3339),
		TemplateData:   contractTemplate.TemplateData,
	}, nil
}

// run policy, schema, and semantic validations; return findings.
func (s *templateRepositorysrvc) Verify(ctx context.Context, req *templaterepository.ContractTemplateVerifyRequest) (res *templaterepository.ContractTemplateVerifyResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.VerifyCmd{
		DID:        req.Did,
		UpdatedAt:  updatedAt,
		VerifiedBy: middleware.GetUsername(ctx),
	}
	handler := command.Verifier{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateVerifyResponse{
		Did: req.Did,
	}, nil
}

// mark template as approved, with optional decision notes.
func (s *templateRepositorysrvc) Approve(ctx context.Context, req *templaterepository.ContractTemplateApproveRequest) (res *templaterepository.ContractTemplateApproveResponse, err error) {

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
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateApproveResponse{
		Did: req.Did,
	}, nil
}

// mark template as rejected, requiring reason field.
func (s *templateRepositorysrvc) Reject(ctx context.Context, req *templaterepository.ContractTemplateRejectRequest) (res *templaterepository.ContractTemplateRejectResponse, err error) {

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
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRejectResponse{
		Did: req.Did,
	}, nil
}

// register new template into the repository.
func (s *templateRepositorysrvc) Register(ctx context.Context, req *templaterepository.ContractTemplateRegisterRequest) (res *templaterepository.ContractTemplateRegisterResponse, err error) {

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.RegisterCmd{
		DID:          req.Did,
		UpdatedAt:    updatedAt,
		RegisteredBy: middleware.GetUsername(ctx),
	}
	handler := command.Registrar{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRegisterResponse{
		Did: req.Did,
	}, nil
}

// archive obsolete template.
func (s *templateRepositorysrvc) Archive(ctx context.Context, req *templaterepository.ContractTemplateArchiveRequest) (res *templaterepository.ContractTemplateArchiveResponse, err error) {

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
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateArchiveResponse{
		Did: req.Did,
	}, nil
}

// retrieve audit history of template actions.
func (s *templateRepositorysrvc) Audit(ctx context.Context, req *templaterepository.ContractTemplateAuditRequest) (res *templaterepository.ContractTemplateAuditResponse, err error) {

	cmd := command.AuditCmd{
		DID:       req.Did,
		AuditedBy: middleware.GetUsername(ctx),
	}
	handler := command.Auditor{
		Ctx:    ctx,
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateAuditResponse{
		Did: req.Did,
	}, nil
}
