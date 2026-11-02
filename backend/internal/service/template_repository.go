package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/templatestructure"
	"digital-contracting-service/internal/base/validation"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	qry2 "digital-contracting-service/internal/processauditandcompliance/query"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"digital-contracting-service/internal/templaterepository/query"

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
	"digital-contracting-service/internal/templaterepository/datatype/partytype"
	"digital-contracting-service/internal/templaterepository/db"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"

	"github.com/jmoiron/sqlx"
	goa "goa.design/goa/v3/pkg"
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
	// Credentials verifies the provenance credentials on /template/verify (DCS-FR-TR-09, DCS-FR-TR-20).
	Credentials *provenance.CredentialVerifier
	auth.JWTAuthenticator
}

// NewTemplateRepository returns the TemplateRepository service implementation.
func NewTemplateRepository(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, CTRepo db.ContractTemplateRepo,
	RTRepo db.ReviewTaskRepo, ATRepo db.ApprovalTaskRepo, fcClient *fcclient.FederatedCatalogueClient,
	auditTrailReader base.AuditTrailReader, vcSigner provenance.VCSigner, issuerDID string, credentials *provenance.CredentialVerifier) templaterepository.Service {
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
		Credentials:      credentials,
	}
}

// mapTemplateCommandError maps the errors a client causes (invalid input, a
// state that does not allow the command, a failed policy audit, an unknown
// template) to bad_request and a stale view to conflict; everything else
// stays an internal error (DCS-IR-TR-04).
func mapTemplateCommandError(err error) error {
	if errors.Is(err, validation.ErrDocumentSchemaConflict) || errors.Is(err, command.ErrInvalidTemplateState) ||
		errors.Is(err, templatestructure.ErrInvalid) || errors.Is(err, command.ErrTemplateNotCompliant) ||
		errors.Is(err, db.ErrContractTemplateNotFound) || errors.Is(err, command.ErrInvalidRequest) ||
		errors.Is(err, command.ErrTemplateInUse) {
		return templaterepository.MakeBadRequest(err)
	}
	// Same optimistic-concurrency refusal the contract commands answer with a
	// retryable conflict: the caller re-reads and reissues, so reporting it as
	// an internal error tells it the opposite of the truth.
	if errors.Is(err, base.ErrUpdatedElsewhere) {
		return goa.NewServiceError(err, "conflict", false, true, false)
	}
	return templaterepository.MakeInternalError(err)
}

func (s *templateRepositorysrvc) Create(ctx context.Context, req *templaterepository.ContractTemplateCreateRequest) (*templaterepository.ContractTemplateCreateResponse, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	partyType, err := partytype.New(req.PartyType)
	if err != nil {
		return nil, goa.NewServiceError(err, "bad_request", false, false, false)
	}
	templateType, err := contracttemplatetype.NewContractTemplateType(req.TemplateType)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
	}

	templateData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
	}

	did, err := base.GenerateID()
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	cmd := command.CreateCmd{
		DID:               *did,
		CreatedBy:         middleware.GetParticipantID(ctx),
		HolderDID:         middleware.GetHolderDID(ctx),
		TemplateType:      templateType,
		PartyType:         partyType.String(),
		Name:              req.Name,
		Description:       req.Description,
		TemplateData:      &templateData,
		Jurisdiction:      req.Jurisdiction,
		Industry:          req.Industry,
		TemplateLevel:     base.DerefString(req.TemplateLevel),
		RequiredTemplates: req.RequiredTemplates,
		UserRoles:         middleware.GetUserRoles(ctx),
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
		return nil, templaterepository.MakeBadRequest(err)
	}

	var actionFlag *actionflag.ActionFlag
	if req.ForwardTo != nil {
		flag, err := actionflag.NewActionFlag(*req.ForwardTo)
		if err != nil {
			return nil, templaterepository.MakeBadRequest(err)
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
		return nil, templaterepository.MakeBadRequest(err)
	}

	metaData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
	}

	var partyType partytype.PartyType
	if req.PartyType != nil {
		var err error
		partyType, err = partytype.New(*req.PartyType)
		if err != nil {
			return nil, goa.NewServiceError(err, "bad_request", false, false, false)
		}
	}
	var templateType *contracttemplatetype.ContractTemplateType
	if req.TemplateType != nil {
		tType, err := contracttemplatetype.NewContractTemplateType(*req.TemplateType)
		if err != nil {
			return nil, templaterepository.MakeBadRequest(err)
		}
		templateType = &tType
	}

	cmd := command.UpdateCmd{
		DID:               req.Did,
		UpdatedAt:         updatedAt,
		TemplateType:      templateType,
		PartyType:         partyType.String(),
		Name:              req.Name,
		Description:       req.Description,
		TemplateData:      &metaData,
		Jurisdiction:      req.Jurisdiction,
		Industry:          req.Industry,
		TemplateLevel:     req.TemplateLevel,
		RequiredTemplates: req.RequiredTemplates,
		UpdatedBy:         middleware.GetParticipantID(ctx),
		HolderDID:         middleware.GetHolderDID(ctx),
		UserRoles:         middleware.GetUserRoles(ctx),
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

// update metadata.
func (s *templateRepositorysrvc) UpdateManage(ctx context.Context, req *templaterepository.ContractTemplateUpdateManageRequest) (res *templaterepository.ContractTemplateUpdateManageResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
	}

	metaData, err := datatype.NewJSON(req.TemplateData)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
	}

	var partyType partytype.PartyType
	if req.PartyType != nil {
		var err error
		partyType, err = partytype.New(*req.PartyType)
		if err != nil {
			return nil, goa.NewServiceError(err, "bad_request", false, false, false)
		}
	}
	var templateType *contracttemplatetype.ContractTemplateType
	if req.TemplateType != nil {
		tType, err := contracttemplatetype.NewContractTemplateType(*req.TemplateType)
		if err != nil {
			return nil, templaterepository.MakeBadRequest(err)
		}
		templateType = &tType
	}

	cmd := command.UpdateManageCmd{
		DID:               req.Did,
		UpdatedAt:         updatedAt,
		TemplateType:      templateType,
		PartyType:         partyType.String(),
		Name:              req.Name,
		Description:       req.Description,
		TemplateData:      &metaData,
		Jurisdiction:      req.Jurisdiction,
		Industry:          req.Industry,
		TemplateLevel:     req.TemplateLevel,
		RequiredTemplates: req.RequiredTemplates,
		UpdatedBy:         middleware.GetParticipantID(ctx),
		HolderDID:         middleware.GetHolderDID(ctx),
		UserRoles:         middleware.GetUserRoles(ctx),
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

	var partyType partytype.PartyType
	if req.PartyType != nil {
		var err error
		partyType, err = partytype.New(*req.PartyType)
		if err != nil {
			return nil, goa.NewServiceError(err, "bad_request", false, false, false)
		}
	}
	var templateType *contracttemplatetype.ContractTemplateType
	if req.TemplateType != nil {
		value, err := contracttemplatetype.NewContractTemplateType(*req.TemplateType)
		if err != nil {
			return nil, goa.NewServiceError(err, "bad_request", false, false, false)
		}
		templateType = &value
	}
	var state *contracttemplatestate.ContractTemplateState
	if req.State != nil {
		tState, err := contracttemplatestate.NewContractTemplateState(*req.State)
		if err != nil {
			return nil, templaterepository.MakeBadRequest(err)
		}

		state = &tState
	}

	pagination := datatype.Pagination{
		Offset: base.DerefInt(req.Offset),
		Limit:  base.DerefInt(req.Limit),
	}

	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy:  middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		DID:          base.DerefString(req.Did),
		Version:      base.DerefInt(req.Version),
		State:        state,
		TemplateType: templateType,
		PartyType:    partyType.String(),
		Name:         base.DerefString(req.Name),
		Description:  base.DerefString(req.Description),
		TemplateData: base.DerefString(req.TemplateData),
		Pagination:   pagination,

		Jurisdiction:     base.DerefString(req.Jurisdiction),
		Industry:         base.DerefString(req.Industry),
		ComplianceStatus: base.DerefString(req.ComplianceStatus),
		CreatedBy:        base.DerefString(req.CreatedBy),
		CreatedFrom:      base.DerefString(req.CreatedFrom),
		CreatedTo:        base.DerefString(req.CreatedTo),
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
			Did:               item.DID,
			Version:           item.Version,
			State:             item.State.String(),
			TemplateType:      item.TemplateType.String(),
			PartyType:         item.PartyType,
			Name:              item.Name,
			Description:       item.Description,
			CreatedBy:         item.CreatedBy,
			CreatedAt:         item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:         item.UpdatedAt.Format(time.RFC3339),
			Jurisdiction:      item.Jurisdiction,
			Industry:          item.Industry,
			ComplianceStatus:  &item.ComplianceStatus,
			TemplateLevel:     &item.TemplateLevel,
			RequiredTemplates: item.RequiredTemplates,
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
		return nil, templaterepository.MakeInternalError(err)
	}

	var contractTemplates []*templaterepository.ContractTemplateHistoryRetrieveByIDResponse
	for _, item := range result {

		contractTemplates = append(contractTemplates, &templaterepository.ContractTemplateHistoryRetrieveByIDResponse{
			Did:          item.DID,
			Version:      item.Version,
			State:        item.State.String(),
			Name:         item.Name,
			Description:  item.Description,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
			TemplateData: item.TemplateData,
			TemplateType: item.TemplateType.String(),
			PartyType:    item.PartyType,

			Jurisdiction:      item.Jurisdiction,
			Industry:          item.Industry,
			ComplianceStatus:  &item.ComplianceStatus,
			TemplateLevel:     &item.TemplateLevel,
			RequiredTemplates: item.RequiredTemplates,
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
			Did:          item.DID,
			Version:      item.Version,
			State:        item.State.String(),
			TemplateType: item.TemplateType.String(),
			PartyType:    item.PartyType,
			Name:         item.Name,
			Description:  item.Description,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
			LatestDid:    item.LatestDID,

			Jurisdiction:      item.Jurisdiction,
			Industry:          item.Industry,
			ComplianceStatus:  &item.ComplianceStatus,
			TemplateLevel:     &item.TemplateLevel,
			RequiredTemplates: item.RequiredTemplates,
		})
	}

	var reviewTasks []*templaterepository.ReviewTaskItem
	for _, item := range result.ReviewerTasks {
		reviewTasks = append(reviewTasks, &templaterepository.ReviewTaskItem{
			Did:       item.DID,
			Version:   item.Version,
			Reviewer:  item.Reviewer,
			State:     item.State.String(),
			CreatedAt: item.CreatedAt.Format(time.RFC3339),
		})
	}

	var approvalTasks []*templaterepository.ApprovalTaskItem
	for _, item := range result.ApprovalTasks {
		approvalTasks = append(approvalTasks, &templaterepository.ApprovalTaskItem{
			Did:       item.DID,
			Version:   item.Version,
			State:     item.State.String(),
			Approver:  item.Approver,
			CreatedAt: item.CreatedAt.Format(time.RFC3339),
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
	if errors.Is(err, db.ErrContractTemplateNotFound) {
		return nil, templaterepository.MakeNotFound(err)
	}
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}
	var notes []*templaterepository.TemplateReviewNote
	for _, note := range contractTemplate.ReviewNotes {
		notes = append(notes, &templaterepository.TemplateReviewNote{
			Kind:      note.Kind,
			Text:      note.Text,
			By:        &note.By,
			CreatedAt: note.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	templateJSONLD, err := semanticmapper.BuildTemplateJSONLD(db.ContractTemplate{
		DID:          contractTemplate.DID,
		Version:      contractTemplate.Version,
		State:        contractTemplate.State.String(),
		TemplateType: contractTemplate.TemplateType.String(),
		PartyType:    contractTemplate.PartyType,
		Name:         contractTemplate.Name,
		Description:  contractTemplate.Description,
		CreatedBy:    contractTemplate.CreatedBy,
		CreatedAt:    contractTemplate.CreatedAt,
		UpdatedAt:    contractTemplate.UpdatedAt,
		TemplateData: contractTemplate.TemplateData,
	})
	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	return &templaterepository.ContractTemplateRetrieveByIDResponse{
		Did:          contractTemplate.DID,
		Version:      contractTemplate.Version,
		State:        contractTemplate.State.String(),
		TemplateType: contractTemplate.TemplateType.String(),
		PartyType:    contractTemplate.PartyType,
		Name:         contractTemplate.Name,
		Description:  contractTemplate.Description,
		CreatedBy:    contractTemplate.CreatedBy,
		CreatedAt:    contractTemplate.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    contractTemplate.UpdatedAt.Format(time.RFC3339),
		TemplateData: templateJSONLD,

		Jurisdiction:      contractTemplate.Jurisdiction,
		Industry:          contractTemplate.Industry,
		ComplianceStatus:  &contractTemplate.ComplianceStatus,
		TemplateLevel:     &contractTemplate.TemplateLevel,
		RequiredTemplates: contractTemplate.RequiredTemplates,
		ReviewNotes:       notes,
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
		DB:               s.DB,
		CTRepo:           s.CTRepo,
		RTRepo:           s.RTRepo,
		FCClient:         s.FCClient,
		VerifyCredential: s.Credentials.Verify,
	}
	result, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, mapTemplateCommandError(err)
	}

	return &templaterepository.ContractTemplateVerifyResponse{
		Did:      req.Did,
		Findings: result.Findings,
		Warnings: result.Warnings,
	}, nil
}

// VerifyByID serves GET /template/verify/{did} with the checks and audit of Verify (DCS-FR-TR-20).
func (s *templateRepositorysrvc) VerifyByID(ctx context.Context, req *templaterepository.ContractTemplateVerifyRequest) (*templaterepository.ContractTemplateVerifyResponse, error) {
	return s.Verify(ctx, req)
}

// mark template as approved, with optional decision notes.
func (s *templateRepositorysrvc) Approve(ctx context.Context, req *templaterepository.ContractTemplateApproveRequest) (res *templaterepository.ContractTemplateApproveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	updatedAt, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, templaterepository.MakeBadRequest(err)
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
		return nil, templaterepository.MakeBadRequest(err)
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
		FCClient:  s.FCClient,
		VCSigner:  s.VCSigner,
		IssuerDID: s.IssuerDID,
	}
	did, err := handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapTemplateCommandError(err)
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
	// DCS-FR-TR-21: a provenance read is logged like any other template read.
	evt := templateevents.RetrieveProvenanceEvent{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		OccurredAt:  time.Now().UTC(),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo); err != nil {
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
		return nil, templaterepository.MakeBadRequest(err)
	}

	cmd := command.ArchiveCmd{
		DID:           req.Did,
		UpdatedAt:     updatedAt,
		ArchivedBy:    middleware.GetParticipantID(ctx),
		Justification: req.Justification,
		HolderDID:     middleware.GetHolderDID(ctx),
		UserRoles:     middleware.GetUserRoles(ctx),
	}
	handler := command.Archiver{
		DB:       s.DB,
		CTRepo:   s.CTRepo,
		RTRepo:   s.RTRepo,
		ATRepo:   s.ATRepo,
		FCClient: s.FCClient,
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
		Scope:         componenttype.ContractTemplateRepo,
		DID:           req.Did,
		AuditedBy:     middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		UserRoles:     middleware.GetUserRoles(ctx),
		Justification: req.Justification,
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
		return nil, templaterepository.MakeBadRequest(err)
	}

	cmd := command.PublishCmd{
		DID:         req.Did,
		UpdatedAt:   updatedAt,
		PublishedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		InstanceDID: s.IssuerDID,
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
