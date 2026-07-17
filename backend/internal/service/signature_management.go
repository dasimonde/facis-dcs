package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/tsa"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/pidverify"
	"digital-contracting-service/internal/signingmanagement/query"
	"digital-contracting-service/internal/signingmanagement/signer"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

// mapSignatureCommandError classifies a signing command error for the HTTP
// layer, mirroring service.mapContractCommandError: a contractstate.
// ErrInvalidTransition (e.g. attempting to sign a contract that isn't
// APPROVED) is a client error (400), everything else stays an internal
// error (500).
func mapSignatureCommandError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, command.ErrCeremonyRequired) || errors.Is(err, command.ErrCeremoniesIncomplete) {
		return signaturemanagement.MakeCeremonyRequired(err)
	}
	if errors.Is(err, contractstate.ErrInvalidTransition) ||
		errors.Is(err, command.ErrUnknownSignatureField) ||
		errors.Is(err, command.ErrFieldAlreadySigned) ||
		errors.Is(err, db.ErrSignatureNotFound) {
		return signaturemanagement.MakeBadRequest(err)
	}
	return signaturemanagement.MakeInternalError(err)
}

type signatureManagementsrvc struct {
	DB            *sqlx.DB
	CRepo         db.ContractRepo
	CeremonyRepo  db.CeremonyRepo
	PDFCore       *pdfcore.Client
	ATrailReader  base.AuditTrailReader
	Signer        signer.ContractSigner
	VCSigner      provenance.VCSigner
	VCIssuer      provenance.VCIssuer
	IssuerDID     string
	IPFSClient    *ipfs.APIClient
	DIDDocument   identity.DIDDocument
	ArchiveRepo   cwedb.ContractRepo
	ArchiveNotary cwecommand.ArchiveNotary
	ArchiveTSA    *tsa.APIClient
	RequestSigner oid4vprequest.Signer
	PublicAPIBase string
	PIDDCQLQuery  any
	auth.JWTAuthenticator
}

func NewSignatureManagement(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, ceremonyRepo db.CeremonyRepo,
	auditTrailReader base.AuditTrailReader, contractSigner signer.ContractSigner, vcSigner provenance.VCSigner, issuerDID string,
	ipfsClient *ipfs.APIClient, pdfCore *pdfcore.Client, archiveRepo cwedb.ContractRepo, archiveNotary cwecommand.ArchiveNotary,
	archiveTSA *tsa.APIClient, vcIssuer provenance.VCIssuer, requestSigner oid4vprequest.Signer, publicAPIBase string, pidDCQLQuery any) signaturemanagement.Service {

	return &signatureManagementsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		CeremonyRepo:     ceremonyRepo,
		PDFCore:          pdfCore,
		ATrailReader:     auditTrailReader,
		Signer:           contractSigner,
		VCSigner:         vcSigner,
		VCIssuer:         vcIssuer,
		IssuerDID:        issuerDID,
		IPFSClient:       ipfsClient,
		ArchiveRepo:      archiveRepo,
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
		RequestSigner:    requestSigner,
		PublicAPIBase:    strings.TrimRight(strings.TrimSpace(publicAPIBase), "/"),
		PIDDCQLQuery:     pidDCQLQuery,
	}
}

func (s *signatureManagementsrvc) Retrieve(ctx context.Context, req *signaturemanagement.SMContractRetrieveRequest) (res *signaturemanagement.SMContractRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	pagination := datatype.Pagination{
		Offset: 0, // DerefInt(req.Offset),
		Limit:  0, // DerefInt(req.Limit),
	}

	qry := query.GetAllMetadataQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
		Pagination:  pagination,
	}
	queryHandler := query.GetAllMetadataHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	var contracts []*signaturemanagement.SMContractListItem
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
			s := item.ExpPolicy.String()
			expPolicy = &s
		}

		contracts = append(contracts, &signaturemanagement.SMContractListItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Name:            item.Name,
			Description:     item.Description,
			CreatedBy:       item.CreatedBy,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
			StartDate:       startDate,
			ExpDate:         expDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: item.ExpNoticePeriod,
			Responsible:     item.Responsible,
		})
	}

	var signingTasks []*signaturemanagement.SMContractSigningTaskItem
	for _, item := range result.SigningTasks {
		signingTasks = append(signingTasks, &signaturemanagement.SMContractSigningTaskItem{
			Did:             item.DID,
			ContractVersion: item.ContractVersion,
			State:           item.State.String(),
			Signer:          item.SignerDID,
			CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		})
	}

	return &signaturemanagement.SMContractRetrieveResponse{
		Contracts:    contracts,
		SigningTasks: signingTasks,
	}, nil
}

func (s *signatureManagementsrvc) RetrieveByID(ctx context.Context, req *signaturemanagement.SMContractRetrieveByIDRequest) (res *signaturemanagement.SMContractRetrieveByIDResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.GetByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	contract := signaturemanagement.SMContractItem{
		Did:             result.Contract.DID,
		ContractVersion: result.Contract.ContractVersion,
		State:           result.Contract.State.String(),
		Name:            result.Contract.Name,
		Description:     result.Contract.Description,
		CreatedAt:       result.Contract.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       result.Contract.UpdatedAt.Format(time.RFC3339),
	}

	signatureEnvelop := &signaturemanagement.SMContractSignatureEnvelope{
		ContractDid:    result.SignatureEnvelope.ContractDID,
		CredentialType: result.SignatureEnvelope.CredentialType,
		IpfsCid:        result.SignatureEnvelope.IpfsCID,
		RevokedAt:      result.SignatureEnvelope.RevokedAt,
		SignedAt:       result.SignatureEnvelope.SignedAt,
		SignerDid:      result.SignatureEnvelope.SignerDID,
		Status:         result.SignatureEnvelope.Status.String(),
	}

	keyVersion := result.SignatureEnvelope.KeyVersion

	return &signaturemanagement.SMContractRetrieveByIDResponse{
		Contract:          &contract,
		SignatureEnvelope: signatureEnvelop,
		KeyVersion:        &keyVersion,
	}, nil
}

func (s *signatureManagementsrvc) Verify(ctx context.Context, req *signaturemanagement.SMContractVerifyRequest) (res *signaturemanagement.SMContractVerifyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.SignatureVerifyQry{
		DID:        req.Did,
		VerifiedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}
	handler := query.SignatureVerifier{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}
	_, err = handler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	return &signaturemanagement.SMContractVerifyResponse{}, nil
}

func (s *signatureManagementsrvc) Apply(ctx context.Context, req *signaturemanagement.SMContractApplyRequest) (res *signaturemanagement.SMContractApplyResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	credentialType := req.CredentialType
	if credentialType == nil || *credentialType == "" {
		aes := "AES"
		credentialType = &aes
	}
	fieldName := ""
	if req.FieldName != nil {
		fieldName = *req.FieldName
	}
	cmd := command.ApplyCmd{
		DID:            req.Did,
		SignerDID:      req.SignerDid,
		FieldName:      fieldName,
		CredentialType: *credentialType,
		AppliedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
	}
	handler := command.Applier{
		DB:            s.DB,
		CRepo:         s.CRepo,
		CeremonyRepo:  s.CeremonyRepo,
		Signer:        s.Signer,
		PDFCore:       s.PDFCore,
		IPFSClient:    s.IPFSClient,
		VCSigner:      s.VCSigner,
		VCIssuer:      s.VCIssuer,
		IssuerDID:     s.IssuerDID,
		ArchiveRepo:   s.ArchiveRepo,
		IPFSStorer:    s.IPFSClient,
		ArchiveNotary: s.ArchiveNotary,
		ArchiveTSA:    s.ArchiveTSA,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}

	qry := query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.GetByIDHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	signatureEnvelop := &signaturemanagement.SMContractSignatureEnvelope{
		ContractDid:    result.SignatureEnvelope.ContractDID,
		CredentialType: result.SignatureEnvelope.CredentialType,
		IpfsCid:        result.SignatureEnvelope.IpfsCID,
		RevokedAt:      result.SignatureEnvelope.RevokedAt,
		SignedAt:       result.SignatureEnvelope.SignedAt,
		SignerDid:      result.SignatureEnvelope.SignerDID,
		Status:         result.SignatureEnvelope.Status.String(),
	}

	return &signaturemanagement.SMContractApplyResponse{
		Did:               req.Did,
		SignatureEnvelope: signatureEnvelop,
	}, nil
}

func (s *signatureManagementsrvc) Validate(ctx context.Context, req *signaturemanagement.SMContractValidateRequest) (res *signaturemanagement.SMContractValidateResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.ValidateQry{
		DID:         req.Did,
		ValidatedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}
	queryHandler := query.Validator{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractValidateResponse{
		Did:      req.Did,
		Findings: result.Findings,
	}, nil
}

func (s *signatureManagementsrvc) Revoke(ctx context.Context, req *signaturemanagement.SMContractRevokeRequest) (res *signaturemanagement.SMContractRevokeResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := command.RevokeCmd{
		DID:       req.Did,
		SignerDID: req.SignerDid,
		RevokedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	queryHandler := command.Revoker{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	err = queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}

	return &signaturemanagement.SMContractRevokeResponse{}, nil
}

func (s *signatureManagementsrvc) Audit(ctx context.Context, req *signaturemanagement.SMContractAuditRequest) (res []*signaturemanagement.SMContractAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := query.GetAuditLogQry{
		DID:       req.Did,
		AuditedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	handler := query.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	auditLogHistory, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	history := make([]*signaturemanagement.SMContractAuditResponse, 0)
	for _, entry := range auditLogHistory {
		history = append(history, &signaturemanagement.SMContractAuditResponse{
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

func (s *signatureManagementsrvc) Compliance(ctx context.Context, req *signaturemanagement.SMContractComplianceRequest) (res *signaturemanagement.SMContractComplianceResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := command.ComplianceCmd{
		DID:       req.Did,
		CheckedBy: middleware.GetParticipantID(ctx),
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	queryHandler := command.ComplianceValidator{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	findings, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractComplianceResponse{
		Did:      req.Did,
		Findings: findings,
	}, nil
}

// View serves the Signature Compliance Viewer (DCS-FR-SM-26, DCS-IR-SM-05):
// per-signature signer identity, credential class/signature level, status,
// and timestamps, plus the contract's cryptographic integrity findings from
// the same validation machinery /signature/validate uses.
func (s *signatureManagementsrvc) View(ctx context.Context, req *signaturemanagement.SMSignatureViewRequest) (res *signaturemanagement.SMSignatureViewResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	validator := query.Validator{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}
	validation, err := validator.Handle(ctx, query.ValidateQry{
		DID:         req.Did,
		ValidatedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	processData, err := s.CRepo.ReadProcessDataByDID(ctx, tx, req.Did)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(err)
	}
	records, err := s.CRepo.LoadSignatures(ctx, tx, req.Did)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	signatures := make([]*signaturemanagement.SMSignatureViewItem, 0, len(records))
	for _, rec := range records {
		item := &signaturemanagement.SMSignatureViewItem{
			SignerDid:      rec.SignerDID,
			FieldName:      rec.FieldName,
			CredentialType: rec.CredentialType,
			Status:         rec.Status,
			Format:         "PAdES (ETSI.CAdES.detached)",
		}
		if rec.SignedAt != nil {
			t := rec.SignedAt.UTC().Format(time.RFC3339)
			item.SignedAt = &t
		}
		if rec.RevokedAt != nil {
			t := rec.RevokedAt.UTC().Format(time.RFC3339)
			item.RevokedAt = &t
		}
		signatures = append(signatures, item)
	}

	return &signaturemanagement.SMSignatureViewResponse{
		Did:               req.Did,
		ContractState:     processData.State,
		Signatures:        signatures,
		IntegrityFindings: validation.Findings,
	}, nil
}

func (s *signatureManagementsrvc) StartCeremony(ctx context.Context, req *signaturemanagement.SMSignatureRequestStartRequest) (res *signaturemanagement.SMSignatureRequestStartResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.PublicAPIBase == "" {
		log.Printf(ctx, "StartCeremony: DCS_PUBLIC_BASE_URL is empty")
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not start the signing ceremony"))
	}

	handler := command.StartCeremonyHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, command.StartCeremonyCmd{
		ContractDID: req.ContractDid,
		FieldName:   req.FieldName,
		RequestedBy: middleware.GetParticipantID(ctx),
		BaseURL:     s.PublicAPIBase,
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	walletURI := ""
	if ceremony.WalletURI != nil {
		walletURI = *ceremony.WalletURI
	}
	return &signaturemanagement.SMSignatureRequestStartResponse{
		CeremonyID: ceremony.ID,
		WalletURI:  walletURI,
		ExpiresAt:  ceremony.ExpiresAt.Format(time.RFC3339),
		Status:     ceremony.Status,
	}, nil
}

func (s *signatureManagementsrvc) PresentationRequest(ctx context.Context, p *signaturemanagement.PresentationRequestPayload) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	defer func() { _ = tx.Rollback() }()

	ceremony, err := s.CeremonyRepo.GetCeremonyByID(ctx, tx, p.CeremonyID)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony not found"))
	}

	if ceremony.Status != db.CeremonyPending {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony is not pending"))
	}

	if !time.Now().UTC().Before(ceremony.ExpiresAt) {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony expired"))
	}

	if s.RequestSigner == nil || s.PublicAPIBase == "" || s.PIDDCQLQuery == nil {
		log.Printf(ctx, "PresentationRequest: OpenID4VP request signing is not configured")
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not build the authorization request"))
	}

	walletNonce := ""
	if p.WalletNonce != nil {
		walletNonce = strings.TrimSpace(*p.WalletNonce)
	}

	responseURI := s.PublicAPIBase + "/signature/presentation/callback"

	jwt, err := oid4vprequest.BuildJWT(s.RequestSigner, oid4vprequest.Params{
		ClientID:    pidverify.Audience,
		ResponseURI: responseURI,
		State:       ceremony.ID,
		Nonce:       ceremony.Nonce,
		WalletNonce: walletNonce,
		ExpiresAt:   ceremony.ExpiresAt,
		DCQLQuery:   s.PIDDCQLQuery,
	})

	if err != nil {
		log.Printf(ctx, "PresentationRequest: build JAR failed: %v", err)
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not build the authorization request"))
	}

	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

func (s *signatureManagementsrvc) CeremonyStatus(ctx context.Context, req *signaturemanagement.SMSignatureRequestStatusRequest) (res *signaturemanagement.SMSignatureRequestStatusResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := query.CeremonyStatusHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, query.CeremonyStatusQry{CeremonyID: req.CeremonyID})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", req.CeremonyID))
	}

	res = &signaturemanagement.SMSignatureRequestStatusResponse{
		CeremonyID: ceremony.ID,
		Status:     ceremony.Status,
	}
	res.ContractDid = &ceremony.ContractDID
	res.FieldName = &ceremony.FieldName
	res.SignerDid = ceremony.SignerDID
	expiresAt := ceremony.ExpiresAt.Format(time.RFC3339)
	res.ExpiresAt = &expiresAt
	return res, nil
}

func (s *signatureManagementsrvc) CeremonyWebhook(ctx context.Context, req *signaturemanagement.SMSignatureWebhookRequest) (res *signaturemanagement.SMSignatureWebhookResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	secret := ""
	if req.WebhookSecret != nil {
		secret = *req.WebhookSecret
	}
	handler := command.WebhookHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, command.WebhookCmd{
		Secret:     secret,
		CeremonyID: req.CeremonyID,
		VpToken:    req.VpToken,
		PidClaims:  req.PidClaims,
	})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrWebhookUnauthorized):
			return nil, signaturemanagement.MakeUnauthorized(err)
		case errors.Is(err, command.ErrCeremonyNotFound):
			return nil, signaturemanagement.MakeNotFound(err)
		default:
			return nil, signaturemanagement.MakeInternalError(err)
		}
	}

	return &signaturemanagement.SMSignatureWebhookResponse{
		CeremonyID: ceremony.ID,
		Status:     ceremony.Status,
	}, nil
}
