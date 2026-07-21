package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/validation"

	"digital-contracting-service/internal/base/datatype/componenttype"
	qry2 "digital-contracting-service/internal/processauditandcompliance/query"

	"digital-contracting-service/internal/templaterepository/query"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"

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
	// VCSigner + IssuerDID issue the per-version template provenance VCs at
	// registration (DCS-FR-TR-09, command/provenance.go).
	VCSigner  provenance.VCSigner
	IssuerDID string
	auth.JWTAuthenticator
}

// NewTemplateRepository returns the TemplateRepository service implementation.
func NewTemplateRepository(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, CTRepo db.ContractTemplateRepo,
	RTRepo db.ReviewTaskRepo, ATRepo db.ApprovalTaskRepo, fcClient *fcclient.FederatedCatalogueClient,
	auditTrailReader base.AuditTrailReader, vcSigner provenance.VCSigner, issuerDID string) templaterepository.Service {
	return &templateRepositorysrvc{
		DB:               db,
		JWTAuthenticator: jwtAuth,
		CTRepo:           CTRepo,
		RTRepo:           RTRepo,
		ATRepo:           ATRepo,
		FCClient:         fcClient,
		ATrailReader:     auditTrailReader,
		VCSigner:         vcSigner,
		IssuerDID:        issuerDID,
	}
}

// Create a new template.
// mapTemplateCommandError maps a client-input document error (a Semantic Hub
// ontology-prefix conflict, DCS-FR-TR-03) to bad_request; everything else
// stays an internal error.
func mapTemplateCommandError(err error) error {
	if errors.Is(err, validation.ErrDocumentSchemaConflict) {
		return templaterepository.MakeBadRequest(err)
	}
	return templaterepository.MakeInternalError(err)
}

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

	did, err := base.GenerateID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.CreateCmd{
		DID:            *did,
		CreatedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		TemplateType:   templateType,
		Name:           req.Name,
		Description:    req.Description,
		DocumentNumber: req.DocumentNumber,
		TemplateData:   &templateData,
		UserRoles:      middleware.GetUserRoles(ctx),
	}
	createHandler := command.Creator{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
	}

	return &templaterepository.ContractTemplateCreateResponse{
		Did: *did,
	}, nil
}

// Copy a new template.
func (s *templateRepositorysrvc) Copy(ctx context.Context, req *templaterepository.ContractTemplateCopyRequest) (*templaterepository.ContractTemplateCopyResponse, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	did, err := base.GenerateID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.CopyCmd{
		NewDID:    *did,
		CopyDID:   req.Did,
		CopiedBy:  middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	copyHandler := command.Copier{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	err = copyHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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
		SubmittedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		ActionFlag:  actionFlag,
		Comments:    req.Comments,
	}
	handler := command.Submitter{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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
		UpdatedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
	}
	handler := command.Updater{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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
		UpdatedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
	}
	handler := command.UpdateManager{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy:    middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
		DID:            base.DerefString(req.Did),
		DocumentNumber: base.DerefString(req.DocumentNumber),
		Version:        base.DerefInt(req.Version),
		State:          state,
		Name:           base.DerefString(req.Name),
		Description:    base.DerefString(req.Description),
		TemplateData:   base.DerefString(req.TemplateData),
		Pagination:     pagination,
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

func (s *templateRepositorysrvc) RetrieveHistoryByID(ctx context.Context, req *templaterepository.ContractTemplateHistoryRetrieveByIDRequest) (res []*templaterepository.ContractTemplateHistoryRetrieveByIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contracttemplate.GetHistoryByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
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
			Did:            item.DID,
			DocumentNumber: item.DocumentNumber,
			Version:        item.Version,
			State:          item.State.String(),
			Name:           item.Name,
			Description:    item.Description,
			CreatedBy:      item.CreatedBy,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
			TemplateData:   item.TemplateData,
			TemplateType:   item.TemplateType.String(),
		})
	}

	return contractTemplates, nil
}

// retrieve templates
func (s *templateRepositorysrvc) Retrieve(ctx context.Context, req *templaterepository.ContractTemplateRetrieveRequest) (res *templaterepository.ContractTemplateRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contracttemplate.GetAllMetadataQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		Pagination:  pagination,
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
			LatestDid:      item.LatestDID,
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
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}
	templateJSONLD, err := semanticmapper.BuildTemplateJSONLD(db.ContractTemplate{
		DID:            contractTemplate.DID,
		DocumentNumber: contractTemplate.DocumentNumber,
		Version:        contractTemplate.Version,
		State:          contractTemplate.State.String(),
		TemplateType:   contractTemplate.TemplateType.String(),
		Name:           contractTemplate.Name,
		Description:    contractTemplate.Description,
		CreatedBy:      contractTemplate.CreatedBy,
		CreatedAt:      contractTemplate.CreatedAt,
		UpdatedAt:      contractTemplate.UpdatedAt,
		TemplateData:   contractTemplate.TemplateData,
	})
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
		TemplateData:   templateJSONLD,
	}, nil
}

// run policy, schema, and semantic validations; return findings.
func (s *templateRepositorysrvc) Verify(ctx context.Context, req *templaterepository.ContractTemplateVerifyRequest) (res *templaterepository.ContractTemplateVerifyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.VerifyQry{
		DID:           req.Did,
		VerifiedBy:    middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		UserRoles:     middleware.GetUserRoles(ctx),
		ParticipantID: middleware.GetParticipantID(ctx),
	}
	handler := query.Verifier{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		RTRepo:   s.RTRepo,
		FCClient: s.FCClient,
	}
	result, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateVerifyResponse{
		Did:      req.Did,
		Findings: result.Findings,
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
		ApprovedBy:    middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		UserRoles:     middleware.GetUserRoles(ctx),
		DecisionNotes: req.DecisionNotes,
	}
	handler := command.Approver{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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
		RejectedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
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
		return nil, mapTemplateCommandError(err)
	}

	return &templaterepository.ContractTemplateRejectResponse{
		Did: req.Did,
	}, nil
}

// register new template into the repository.
func (s *templateRepositorysrvc) Register(ctx context.Context, req *templaterepository.ContractTemplateRegisterRequest) (res *templaterepository.ContractTemplateRegisterResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	cmd := command.RegisterCmd{
		DID:          req.Did,
		Version:      base.DerefInt(req.Version),
		RegisteredBy: middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
	}
	handler := command.Registrar{
		DB:        s.DB,
		CTRepo:    s.CTRepo,
		RTRepo:    s.RTRepo,
		ATRepo:    s.ATRepo,
		FCClient:  s.FCClient,
		VCSigner:  s.VCSigner,
		IssuerDID: s.IssuerDID,
	}
	did, err := handler.Handle(ctx, cmd)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRegisterResponse{
		Did: *did,
	}, nil
}

// Provenance serves the per-version signed W3C provenance credentials of a
// template (DCS-FR-TR-09) — the artifacts a template user verifies a
// template's creator/reviewer/approver trail with.
func (s *templateRepositorysrvc) Provenance(ctx context.Context, req *templaterepository.TemplateProvenanceRetrieveRequest) (res []*templaterepository.TemplateProvenanceCredentialResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}
	defer func(tx *sqlx.Tx) {
		_ = tx.Rollback()
	}(tx)

	rows, err := s.CTRepo.ReadProvenanceCredentials(ctx, tx, req.Did)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	res = make([]*templaterepository.TemplateProvenanceCredentialResponse, 0, len(rows))
	for _, row := range rows {
		var credential any
		if err := json.Unmarshal(row.Credential, &credential); err != nil {
			return nil, templaterepository.MakeInternalError(fmt.Errorf("decode stored provenance credential %s: %w", row.VCID, err))
		}
		res = append(res, &templaterepository.TemplateProvenanceCredentialResponse{
			Version:      row.Version,
			VcID:         row.VCID,
			PreviousVcID: row.PreviousVCID,
			Credential:   credential,
		})
	}
	return res, nil
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
		ArchivedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := command.Archiver{
		DB:     s.DB,
		CTRepo: s.CTRepo,
		RTRepo: s.RTRepo,
		ATRepo: s.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
	}

	return &templaterepository.ContractTemplateArchiveResponse{
		Did: req.Did,
	}, nil
}

// retrieve audit history of template actions.
func (s *templateRepositorysrvc) Audit(ctx context.Context, req *templaterepository.ContractTemplateAuditRequest) (res []*templaterepository.ContractTemplateAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := qry2.GetAuditLogByDIDQry{
		Scope:     componenttype.ContractTemplateRepo,
		DID:       req.Did,
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

	history := make([]*templaterepository.ContractTemplateAuditResponse, 0)
	for _, entry := range auditLogHistory {
		if !base.IsAuditVisibleEventType(entry.EventType) {
			continue
		}
		history = append(history, &templaterepository.ContractTemplateAuditResponse{
			ID:            entry.ID,
			Component:     entry.Component,
			EventType:     entry.EventType,
			EventData:     entry.EventData,
			Did:           entry.DID,
			CreatedAt:     entry.CreatedAt.String(),
			ResLogPredCid: entry.ResLogPredCID,
		})
	}

	policyTrailQry := qry2.GetTemplatePolicyTrailQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	policyTrailHandler := qry2.ContractTemplatePolicyTrailAuditor{
		DB:     s.DB,
		CTRepo: s.CTRepo,
	}

	policyTrailResult, err := policyTrailHandler.Handle(ctx, policyTrailQry)
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}
	for i, finding := range policyTrailResult {
		did := req.Did
		history = append(history, &templaterepository.ContractTemplateAuditResponse{
			ID:        int64(-1 - i),
			Component: finding.Component,
			EventType: finding.EventType,
			EventData: finding.EventData,
			Did:       &did,
			CreatedAt: finding.CreatedAt.Format(time.RFC3339),
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
		DID:         req.Did,
		UpdatedAt:   updatedAt,
		PublishedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	handler := command.Publisher{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		FCClient: s.FCClient,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
	}

	return &templaterepository.ContractTemplatePublishResponse{
		Did: req.Did,
	}, nil
}

// Resolve dereferences a template's resource IRI: GET /template/{did}
// serves the canonical JSON-LD template document, under the same
// authorization retrieve_by_id enforces.
func (s *templateRepositorysrvc) Resolve(ctx context.Context, req *templaterepository.ContractTemplateRetrieveByIDRequest) (any, error) {
	template, err := s.RetrieveByID(ctx, req)
	if err != nil {
		return nil, err
	}
	return template.TemplateData, nil
}
