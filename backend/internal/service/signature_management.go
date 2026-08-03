package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/tsa"
	"digital-contracting-service/internal/base/validation"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	"digital-contracting-service/internal/signingmanagement/query"

	"github.com/jmoiron/sqlx"
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
	// Its own code, not bad_request: "the contract is waiting for the
	// counterparty to settle this version" is a different answer to a signer
	// than "you may not sign", and the frontend distinguishes signing
	// refusals by code, never by matching the message.
	if errors.Is(err, command.ErrCounterpartyNotSettled) {
		return signaturemanagement.MakeCounterpartyNotSettled(err)
	}
	// Every ADR-20 acceptance-gate rejection gets its OWN typed Goa error, not
	// a shared signature_invalid — the frontend's validation-failure view
	// (item 11) distinguishes these by CODE, never by matching error text.
	switch {
	// All three halves of "the submission is the document we prepared" report
	// the same client-facing code: ErrDocumentMismatch is the append-only half,
	// ErrContentMismatch the visible-content half, ErrPayloadMismatch the
	// machine-readable half, and the message carries which one fired and — for
	// the latter two — the page that diverged or the two payload digests.
	case errors.Is(err, command.ErrDocumentMismatch), errors.Is(err, command.ErrContentMismatch),
		errors.Is(err, command.ErrPayloadMismatch):
		return signaturemanagement.MakeDocumentMismatch(err)
	case errors.Is(err, command.ErrNonceMismatch):
		return signaturemanagement.MakeNonceMismatch(err)
	case errors.Is(err, command.ErrLevelBelowRequired):
		return signaturemanagement.MakeLevelBelowRequired(err)
	case errors.Is(err, command.ErrCertPIDMismatch), errors.Is(err, command.ErrCertInconsistent):
		return signaturemanagement.MakeCertPidMismatch(err)
	case errors.Is(err, command.ErrJAdESInvalid):
		return signaturemanagement.MakeJadesInvalid(err)
	case errors.Is(err, command.ErrSignatureInvalid):
		return signaturemanagement.MakeSignatureInvalid(err)
	}
	if errors.Is(err, contractstate.ErrInvalidTransition) ||
		errors.Is(err, command.ErrRevocationReasonRequired) ||
		errors.Is(err, command.ErrUnknownSignatureField) ||
		errors.Is(err, command.ErrFieldAlreadySigned) ||
		errors.Is(err, command.ErrCeremonyNotPrepared) ||
		errors.Is(err, command.ErrCeremonyConsumed) ||
		errors.Is(err, validation.ErrContractNotClosed) ||
		errors.Is(err, db.ErrSignatureNotFound) {
		return signaturemanagement.MakeBadRequest(err)
	}
	return signaturemanagement.MakeInternalError(err)
}

type signatureManagementsrvc struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	CeremonyRepo db.CeremonyRepo
	PDFCore      *pdfcore.Client
	ATrailReader base.AuditTrailReader
	VCSigner     provenance.VCSigner
	VCIssuer     provenance.VCIssuer
	IssuerDID    string
	// DIDDocument identifies this instance as a PEER — the identity the
	// DCS-to-DCS channel writes into the settlement artifacts the signing gate
	// reads, and the one that tells the counterparty's signature slot from
	// ours. Distinct from IssuerDID, which is configured separately.
	DIDDocument identity.DIDDocument
	// Settlements is the cross-instance sync store holding the settlement
	// artifacts: the counterparties' and this instance's own.
	Settlements   command.PeerSettlements
	Artifacts     *artifactstore.Store
	ArchiveRepo   cwedb.ContractRepo
	ArchiveNotary cwecommand.ArchiveNotary
	ArchiveTSA    *tsa.APIClient
	WorkflowGate  *workflowgate.Coordinator
	// RequestSigner signs both request objects a ceremony publishes — the
	// pending-stage PID/PoA presentation request and the Document-Retrieval
	// request — with the DCS's own certificate chain in the header, the same
	// signer the login flow uses. A wallet verifies either against the SAN the
	// client identifier names.
	RequestSigner oid4vprequest.Signer
	// OID4VPClientID is the prefixed x509_san_dns client identifier BOTH request
	// objects a ceremony publishes declare — the pending PID/PoA presentation
	// request and the Document-Retrieval request — and therefore the audience the
	// presented KB-JWTs must be bound to. One ceremony is reached through one
	// request_uri, so it must name one verifier.
	OID4VPClientID string
	// PublicAPIBase is the externally-resolvable API base the request object's
	// request_uri, document_locations, and response_uri are built from.
	PublicAPIBase string
	// PIDDCQLQuery is the DCQL query for the PID credential a pending signing
	// ceremony's request object asks the wallet to present. Same value the auth
	// service loads from OID4VP_PID_DCQL_QUERY for the PID login flow.
	PIDDCQLQuery any
	// DCQLQuery is the DCQL query for the PoA credential merged into that same
	// ceremony request object alongside PIDDCQLQuery. Same value the auth service
	// loads from OID4VP_DCQL_QUERY for login.
	DCQLQuery any
	// Trust is the issuer trust configuration used to verify PID and PoA
	// presentations at the ceremony callback. Same trust anchors the auth login
	// and PID-verify flows use.
	Trust *oid4vp.TrustConfig
	// Credentials verifies a credential read out of a stored PDF — a signing
	// summary, a lifecycle credential — against the key its issuer publishes for
	// assertions, before anything it claims is used.
	Credentials *provenance.CredentialVerifier
	// CredentialStatus resolves that credential's revocation entry against the
	// signed status list it names.
	CredentialStatus *provenance.CredentialStatusVerifier
	// StatusEntries hands out the contract's entry in that list, for the signing
	// summary credentials this service issues.
	StatusEntries provenance.StatusListEntries
	auth.JWTAuthenticator
}

func NewSignatureManagement(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, ceremonyRepo db.CeremonyRepo,
	auditTrailReader base.AuditTrailReader, vcSigner provenance.VCSigner, issuerDID string,
	artifacts *artifactstore.Store, pdfCore *pdfcore.Client, archiveRepo cwedb.ContractRepo, archiveNotary cwecommand.ArchiveNotary,
	archiveTSA *tsa.APIClient, vcIssuer provenance.VCIssuer, workflowGate *workflowgate.Coordinator,
	requestSigner oid4vprequest.Signer, oid4vpClientID, publicAPIBase string,
	pidDCQLQuery, dcqlQuery any, trust *oid4vp.TrustConfig,
	credentials *provenance.CredentialVerifier,
	credentialStatus *provenance.CredentialStatusVerifier,
	statusEntries provenance.StatusListEntries,
	didDocument identity.DIDDocument,
	settlements command.PeerSettlements) signaturemanagement.Service {

	// Without it every embedded signing summary would be unverifiable, and the
	// compliance viewer would have nothing it is allowed to report.
	if credentials == nil {
		panic("CredentialVerifier is required to verify embedded signing evidence")
	}
	// Without it a verified credential's revocation entry could not be resolved
	// at all, and a signature would verify against a contract nobody checked was
	// still in force.
	if credentialStatus == nil {
		panic("CredentialStatusVerifier is required to resolve embedded credentials' revocation state")
	}
	service := &signatureManagementsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		CeremonyRepo:     ceremonyRepo,
		PDFCore:          pdfCore,
		ATrailReader:     auditTrailReader,
		VCSigner:         vcSigner,
		VCIssuer:         vcIssuer,
		IssuerDID:        issuerDID,
		Artifacts:        artifacts,
		ArchiveRepo:      archiveRepo,
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
		WorkflowGate:     workflowGate,
		RequestSigner:    requestSigner,
		OID4VPClientID:   oid4vpClientID,
		PublicAPIBase:    publicAPIBase,
		PIDDCQLQuery:     pidDCQLQuery,
		DCQLQuery:        dcqlQuery,
		Trust:            trust,
		Credentials:      credentials,
		CredentialStatus: credentialStatus,
		StatusEntries:    statusEntries,
		DIDDocument:      didDocument,
		Settlements:      settlements,
	}
	if workflowGate != nil {
		workflowGate.SetReviewContinuation("signature", service.resumeReviewedSignatureGate)
	}
	return service
}

func (s *signatureManagementsrvc) resumeReviewedSignatureGate(ctx context.Context, run workflowgate.Run) error {
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
	signedPDF, err := base64.StdEncoding.DecodeString(stringValue("signed_pdf"))
	if err != nil {
		return fmt.Errorf("decode reviewed signed PDF: %w", err)
	}
	applier, err := s.newApplier()
	if err != nil {
		return err
	}
	return applier.SubmitSignature(ctx, command.SubmitSignatureCmd{
		ApplyCmd: command.ApplyCmd{
			DID: stringValue("did"), SignerDID: stringValue("signer_did"),
			FieldName: stringValue("field_name"), CeremonyID: stringValue("ceremony_id"),
			CredentialType: stringValue("credential_type"), AppliedBy: stringValue("requested_by"),
			HolderDID: stringValue("holder_did"), UserRoles: roles,
		},
		SignedPDF: signedPDF, JAdESSignature: stringValue("jades_signature"),
	})
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
		ContractData:    result.Contract.ContractData,
	}

	res = &signaturemanagement.SMContractRetrieveByIDResponse{Contract: &contract}

	// An APPROVED-unsigned contract has no signature envelope yet; the signer
	// still needs to read its content to sign it.
	if envelope := result.SignatureEnvelope; envelope != nil {
		res.SignatureEnvelope = &signaturemanagement.SMContractSignatureEnvelope{
			ContractDid:    envelope.ContractDID,
			CredentialType: envelope.CredentialType,
			IpfsCid:        envelope.IpfsCID,
			RevokedAt:      envelope.RevokedAt,
			SignedAt:       envelope.SignedAt,
			SignerDid:      envelope.SignerDID,
			Status:         envelope.Status.String(),
		}
	}

	return res, nil
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
		DB:               s.DB,
		CRepo:            s.CRepo,
		PDFCore:          s.PDFCore,
		Credentials:      s.Credentials,
		CredentialStatus: s.CredentialStatus,
	}
	result, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	return verifyResponseFrom(req.Did, result), nil
}

// verifyResponseFrom carries the verifier's verdict — the re-render match, the
// active signature count, and the findings that hold the C2PA claim-signature,
// lifecycle-credential and revocation results — onto the wire response.
func verifyResponseFrom(did string, result *query.SignatureVerifyResult) *signaturemanagement.SMContractVerifyResponse {
	res := &signaturemanagement.SMContractVerifyResponse{Did: did}
	if result == nil {
		return res
	}
	res.Match = result.Match
	res.SigCount = result.SigCount
	res.Findings = result.Findings
	res.JsonldHash = result.JsonldHash
	res.BasePdfHash = result.BasePdfHash
	return res
}

func (s *signatureManagementsrvc) Provenance(ctx context.Context, req *signaturemanagement.SMProvenanceRequest) (res *signaturemanagement.SMProvenanceResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := query.ProvenanceChainHandler{
		DB:      s.DB,
		CRepo:   s.CRepo,
		PDFCore: s.PDFCore,
	}
	chain, err := handler.Handle(ctx, req.Did)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	entries := make([]*signaturemanagement.SMProvenanceEntry, 0, len(chain))
	for _, e := range chain {
		entries = append(entries, &signaturemanagement.SMProvenanceEntry{Label: e.Label, Lifecycle: e.Lifecycle})
	}
	return &signaturemanagement.SMProvenanceResponse{Did: req.Did, Chain: entries}, nil
}

// newApplier assembles the signing command handler. Validator is wired from a
// configured DSS (DSS_URL) so SubmitSignature can validate an externally-produced
// signature and confirm it identifies the signatory (sole control, ADR-12).
func (s *signatureManagementsrvc) newApplier() (command.Applier, error) {
	var validator command.SignatureValidator
	if url := dss.URL(); url != "" {
		validator = dss.New(url)
	}
	// The counterparty-settlement gate compares party identities and reads the
	// settlement this instance itself made, both keyed by this did:web. A
	// handler that cannot name itself would silently find no remote party and
	// wave every signature through, so this is fatal rather than empty.
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return command.Applier{}, fmt.Errorf("could not read this instance's own peer identity: %w", err)
	}
	return command.Applier{
		DB:            s.DB,
		CRepo:         s.CRepo,
		CeremonyRepo:  s.CeremonyRepo,
		PDFCore:       s.PDFCore,
		Artifacts:     s.Artifacts,
		VCSigner:      s.VCSigner,
		VCIssuer:      s.VCIssuer,
		IssuerDID:     s.IssuerDID,
		StatusEntries: s.StatusEntries,
		ArchiveRepo:   s.ArchiveRepo,
		IPFSStorer:    s.Artifacts,
		ArchiveNotary: s.ArchiveNotary,
		ArchiveTSA:    s.ArchiveTSA,
		Validator:     validator,
		LocalPeer:     localPeer,
		Settlements:   s.Settlements,
	}, nil
}

// PrepareSignature returns the to-be-signed PDF for the signatory to sign
// externally — with their wallet/QTSP or a desktop PAdES signer. The DCS applies
// no signature (ADR-12, DCS-FR-SM-16).
func (s *signatureManagementsrvc) PrepareSignature(ctx context.Context, req *signaturemanagement.SMSignaturePrepareRequest) (res *signaturemanagement.SMSignaturePrepareResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	credentialType := "AES"
	if req.CredentialType != nil && *req.CredentialType != "" {
		credentialType = *req.CredentialType
	}
	fieldName := ""
	if req.FieldName != nil {
		fieldName = *req.FieldName
	}
	ceremonyID := ""
	if req.CeremonyID != nil {
		ceremonyID = *req.CeremonyID
	}

	handler, err := s.newApplier()
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	document, err := handler.Prepare(ctx, command.ApplyCmd{
		DID:            req.Did,
		SignerDID:      req.SignerDid,
		FieldName:      fieldName,
		CeremonyID:     ceremonyID,
		CredentialType: credentialType,
		AppliedBy:      middleware.GetParticipantID(ctx),
		HolderDID:      middleware.GetHolderDID(ctx),
		UserRoles:      middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}
	return &signaturemanagement.SMSignaturePrepareResponse{Document: document}, nil
}

// SubmitSignature accepts a signature the signatory produced externally and
// finalizes the contract after validating it identifies the signatory (sole
// control, ADR-12, DCS-FR-SM-16/-18). The same path serves the wallet callback
// and a downloaded-then-desktop-signed re-upload.
func (s *signatureManagementsrvc) SubmitSignature(ctx context.Context, req *signaturemanagement.SMSignatureSubmitRequest) (res *signaturemanagement.SMContractApplyResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	credentialType := "AES"
	if req.CredentialType != nil && *req.CredentialType != "" {
		credentialType = *req.CredentialType
	}
	fieldName := ""
	if req.FieldName != nil {
		fieldName = *req.FieldName
	}
	jadesSignature := ""
	if req.JadesSignature != nil {
		jadesSignature = *req.JadesSignature
	}
	ceremonyID := ""
	if req.CeremonyID != nil {
		ceremonyID = *req.CeremonyID
	}

	if s.WorkflowGate != nil {
		if _, _, err := s.WorkflowGate.Execute(ctx, workflowgate.Input{
			Gate: "signature", ContractDID: req.Did,
			Requester: middleware.GetParticipantID(ctx), Roles: workflowRoles(ctx),
			Continuation: map[string]any{
				"did": req.Did, "signer_did": req.SignerDid, "field_name": fieldName,
				"ceremony_id": ceremonyID, "credential_type": credentialType,
				"requested_by":    middleware.GetParticipantID(ctx),
				"holder_did":      middleware.GetHolderDID(ctx),
				"user_roles":      workflowRoles(ctx),
				"signed_pdf":      req.SignedPdf,
				"jades_signature": jadesSignature,
			},
		}); err != nil {
			return nil, err
		}
	}

	handler, err := s.newApplier()
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := handler.SubmitSignature(ctx, command.SubmitSignatureCmd{
		ApplyCmd: command.ApplyCmd{
			DID:            req.Did,
			SignerDID:      req.SignerDid,
			FieldName:      fieldName,
			CeremonyID:     ceremonyID,
			CredentialType: credentialType,
			AppliedBy:      middleware.GetParticipantID(ctx),
			HolderDID:      middleware.GetHolderDID(ctx),
			UserRoles:      middleware.GetUserRoles(ctx),
		},
		SignedPDF:      req.SignedPdf,
		JAdESSignature: jadesSignature,
	}); err != nil {
		return nil, mapSignatureCommandError(err)
	}

	queryHandler := query.GetByIDHandler{DB: s.DB, CRepo: s.CRepo}
	result, err := queryHandler.Handle(ctx, query.GetByIDQry{
		DID:         req.Did,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	envelope := result.SignatureEnvelope
	return &signaturemanagement.SMContractApplyResponse{
		Did: req.Did,
		SignatureEnvelope: &signaturemanagement.SMContractSignatureEnvelope{
			ContractDid:    envelope.ContractDID,
			CredentialType: envelope.CredentialType,
			IpfsCid:        envelope.IpfsCID,
			RevokedAt:      envelope.RevokedAt,
			SignedAt:       envelope.SignedAt,
			SignerDid:      envelope.SignerDID,
			Status:         envelope.Status.String(),
		},
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
		DB:          s.DB,
		CRepo:       s.CRepo,
		PDFCore:     s.PDFCore,
		Credentials: s.Credentials,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)

	}

	return &signaturemanagement.SMContractValidateResponse{
		Did:      req.Did,
		Findings: result.Findings,
		Dss:      mapDSSReport(result.DSSReport),
	}, nil
}

// mapDSSReport lifts the query layer's structured DSS report into the goa
// response type (DCS-FR-SM-18/-26). nil in stays nil out: no DSS configured or
// no signed PDF means no report to render.
func mapDSSReport(r *dss.Report) *signaturemanagement.SMDSSReport {
	if r == nil {
		return nil
	}
	return &signaturemanagement.SMDSSReport{
		Indication:      r.Indication,
		SubIndication:   optString(r.SubIndication),
		SignedBy:        optString(r.SignedBy),
		SignatureFormat: optString(r.SignatureFormat),
		SigningTime:     optString(r.SigningTime),
	}
}

func (s *signatureManagementsrvc) Revoke(ctx context.Context, req *signaturemanagement.SMContractRevokeRequest) (res *signaturemanagement.SMContractRevokeResponse, err error) {
	reason, err := command.NormalizeRevocationReason(req.Reason)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(err)
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := command.RevokeCmd{
		DID:       req.Did,
		SignerDID: req.SignerDid,
		Reason:    reason,
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
		DB:           s.DB,
		CRepo:        s.CRepo,
		CeremonyRepo: s.CeremonyRepo,
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
		DB:          s.DB,
		CRepo:       s.CRepo,
		PDFCore:     s.PDFCore,
		Credentials: s.Credentials,
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
	// Per-signature ceremony lookup (ADR-20 SM-26): the contract's declared
	// level requirement and the signing certificate's subject/serial live on
	// the ceremony row, not the embedded ContractSigningSummaryCredential.
	ceremonies := make(map[string]*db.SignatureCeremony, len(records))
	for _, rec := range records {
		if rec.CeremonyID == nil || ceremonies[*rec.CeremonyID] != nil {
			continue
		}
		ceremony, cErr := s.CeremonyRepo.GetCeremonyByID(ctx, tx, *rec.CeremonyID)
		if cErr != nil {
			return nil, signaturemanagement.MakeInternalError(cErr)
		}
		if ceremony != nil {
			ceremonies[*rec.CeremonyID] = ceremony
		}
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
			Format:         "PAdES (ETSI.CAdES.detached) + JAdES (ETSI TS 119 182-1)",
			Jades:          rec.JAdESSignature,
		}
		qualified := strings.EqualFold(rec.CredentialType, "QES")
		item.Qualified = &qualified
		if rec.SignedAt != nil {
			t := rec.SignedAt.UTC().Format(time.RFC3339)
			item.SignedAt = &t
		}
		if rec.RevokedAt != nil {
			t := rec.RevokedAt.UTC().Format(time.RFC3339)
			item.RevokedAt = &t
		}
		if rec.CeremonyID != nil {
			if ceremony := ceremonies[*rec.CeremonyID]; ceremony != nil {
				item.RequiredCredentialType = ceremony.RequiredCredentialType
				item.SignerCertSubject = ceremony.SignerCertSubject
				item.SignerCertSerial = ceremony.SignerCertSerial
			}
		}
		enrichWithSigningEvidence(item, rec, validation.SigningEvidence)
		signatures = append(signatures, item)
	}

	return &signaturemanagement.SMSignatureViewResponse{
		Did:               req.Did,
		ContractState:     processData.State,
		Signatures:        signatures,
		IntegrityFindings: validation.Findings,
		Dss:               mapDSSReport(validation.DSSReport),
	}, nil
}

// enrichWithSigningEvidence attaches the integrity proof and credential binding
// from the signature's embedded ContractSigningSummaryCredential (DCS-FR-SM-26).
// Evidence is matched to the signature record by signer DID, disambiguated by
// the declared field on multi-signer contracts. A signature whose evidence is
// absent (e.g. pre-evidence data) simply carries no proof fields.
func enrichWithSigningEvidence(item *signaturemanagement.SMSignatureViewItem, rec db.SignatureRecord, evidence []query.SigningEvidence) {
	field := ""
	if rec.FieldName != nil {
		field = *rec.FieldName
	}
	for i := range evidence {
		ev := evidence[i]
		if ev.SignerDID != rec.SignerDID {
			continue
		}
		if field != "" && ev.FieldName != "" && ev.FieldName != field {
			continue
		}
		item.CeremonyID = optString(ev.CeremonyID)
		item.ContentHash = optString(ev.ContentHash)
		item.PdfHash = optString(ev.PDFHash)
		item.KbSdHash = optString(ev.KBSDHash)
		item.ValidationReportHash = optString(ev.ValidationReportHash)
		return
	}
}

// optString returns nil for an empty string so optional goa attributes stay
// unset rather than serialising as "".
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *signatureManagementsrvc) StartCeremony(ctx context.Context, req *signaturemanagement.SMSignatureRequestStartRequest) (res *signaturemanagement.SMSignatureRequestStartResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("DCS_PUBLIC_BASE_URL")), "/")
	if baseURL == "" {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not start the signing ceremony"))
	}

	handler := command.StartCeremonyHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	ceremony, err := handler.Handle(ctx, command.StartCeremonyCmd{
		ContractDID: req.ContractDid,
		FieldName:   req.FieldName,
		RequestedBy: middleware.GetParticipantID(ctx),
		BaseURL:     baseURL,
		ClientID:    s.OID4VPClientID,
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

// The EUDIPLO OID4VP webhook (ceremonyWebhook) is removed (ADR-20): the
// remote EUDIPLO PID service is not a dependency of this DCS anymore. Ceremony
// PID+PoA verification runs entirely from the wallet's own direct_post
// (ceremonyPresentationDirectPost in signature_request.go) — see
// command.PresentationHandler.
