package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"goa.design/clue/log"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/signingmanagement/command"
	db "digital-contracting-service/internal/signingmanagement/db"
)

// signingRequestTTL is how long a published OID4VP signing request stays valid
// for a wallet to fetch, sign, and post the signed document back.
const signingRequestTTL = 15 * time.Minute

// signatureQualifierFor maps a DCS credential type to the CSC
// signatureQualifier the wallet honours (the value the EUDI walletdriven-signer
// advertises in the request object). QES is descoped (SRS §199); an unknown type
// defaults to the AES qualifier.
func signatureQualifierFor(credentialType string) string {
	if strings.EqualFold(credentialType, "QES") {
		return "eu_eidas_qes"
	}
	return "eu_eidas_aes"
}

// PublishSignatureRequest runs Applier.Prepare to produce the to-be-signed PDF
// for a verified ceremony, stores it (so the wallet signs exactly the committed
// bytes), and returns the OID4VP Document-Retrieval request as QR/deep-link data
// (ADR-12). The request object itself is served, by reference, from
// GET .../object; the wallet fetches it, fetches the document it references,
// signs, and posts the signed document back to the callback.
func (s *signatureManagementsrvc) PublishSignatureRequest(ctx context.Context, req *signaturemanagement.SMSignatureRequestPublishRequest) (res *signaturemanagement.SMSignatureRequestPublishResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.RequestSigner == nil || strings.TrimSpace(s.OID4VPClientID) == "" {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("OID4VP document-retrieval request signer is not configured"))
	}
	if strings.TrimSpace(s.PublicAPIBase) == "" {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("public API base URL is not configured"))
	}

	ceremony, err := s.getCeremony(ctx, req.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", req.CeremonyID))
	}
	if ceremony.Status != db.CeremonyVerified || ceremony.SignerDID == nil || strings.TrimSpace(*ceremony.SignerDID) == "" {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s has no verified PID presentation to publish a signing request for", req.CeremonyID))
	}

	// Default to the CONTRACT's own declared level requirement for this field
	// (SM-01 per-contract level enforcement) rather than a blind "AES": a
	// caller that omits credential_type is asking the DCS to request whatever
	// the field actually needs, not "AES regardless" — otherwise a QES-required
	// field's publish call fails its own fail-fast check inside Prepare below
	// (comparing "AES" against the QES it itself requires) before the wallet
	// ever gets a chance to sign, and the JAR's signatureQualifier would have
	// asked the wallet for the wrong level anyway. An explicit request still
	// wins (and still meets Prepare's fail-fast check, or is rejected there).
	credentialType := "AES"
	if contractData, cErr := s.readContractDataByDID(ctx, ceremony.ContractDID); cErr == nil && contractData != nil {
		credentialType = validation.RequiredCredentialType(*contractData, ceremony.FieldName)
	}
	if req.CredentialType != nil && *req.CredentialType != "" {
		credentialType = *req.CredentialType
	}

	appliedBy := middleware.GetParticipantID(ctx)
	holderDID := middleware.GetHolderDID(ctx)
	roles := middleware.GetUserRoles(ctx)
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("encode signer roles: %w", err))
	}

	// Prepare seals the agreement, embeds the signing-summary evidence, and
	// places the AcroForm field, yielding the to-be-signed PDF (it holds no
	// signing key). This is the exact same preparation /signature/prepare runs.
	applier := s.newApplier()
	document, err := applier.Prepare(ctx, command.ApplyCmd{
		DID:            ceremony.ContractDID,
		SignerDID:      *ceremony.SignerDID,
		FieldName:      ceremony.FieldName,
		CredentialType: credentialType,
		AppliedBy:      appliedBy,
		HolderDID:      holderDID,
		UserRoles:      roles,
		// Pin the EXACT ceremony this publish call resolved above (ceremony.ID
		// from req.CeremonyID), not "the signer's most recent verified ceremony
		// for this field" — the same ambiguity SubmitSignature's callback path
		// closes below, here on the prepare/pin side.
		CeremonyID: ceremony.ID,
	})
	if err != nil {
		return nil, mapSignatureCommandError(err)
	}

	sum := sha256.Sum256(document)
	digestHex := hex.EncodeToString(sum[:])
	nonce := uuid.NewString()
	expiresAt := time.Now().UTC().Add(signingRequestTTL)

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.CeremonyRepo.StorePreparedRequest(ctx, tx, db.PreparedRequest{
		CeremonyID:        ceremony.ID,
		PreparedPDF:       document,
		PreparedPDFSHA256: digestHex,
		RequestNonce:      nonce,
		RequestExpiresAt:  expiresAt,
		CredentialType:    credentialType,
		PublishedBy:       appliedBy,
		HolderDID:         holderDID,
		Roles:             rolesJSON,
	}); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	requestURI := s.signatureRequestURL(ceremony.ID, "object")
	return &signaturemanagement.SMSignatureRequestPublishResponse{
		CeremonyID: ceremony.ID,
		ClientID:   s.OID4VPClientID,
		RequestURI: requestURI,
		WalletURI:  buildOpenID4VPPresentationURI(s.OID4VPClientID, requestURI),
		Nonce:      &nonce,
		ExpiresAt:  expiresAt.Format(time.RFC3339),
	}, nil
}

// assertPreparedDocumentDigestConsistent verifies the ceremony's persisted
// PreparedPDF and PreparedPDFSHA256 still agree, at every point either is
// about to be handed to a wallet (JAR digest and document serving).
func assertPreparedDocumentDigestConsistent(ceremony *db.SignatureCeremony) error {
	if ceremony.PreparedPDFSHA256 == nil {
		return fmt.Errorf("ceremony %s has no pinned document digest", ceremony.ID)
	}
	actual := sha256.Sum256(ceremony.PreparedPDF)
	if hex.EncodeToString(actual[:]) != strings.ToLower(strings.TrimSpace(*ceremony.PreparedPDFSHA256)) {
		return fmt.Errorf("ceremony %s: prepared document no longer matches its pinned digest", ceremony.ID)
	}
	return nil
}

// SignatureRequestObject serves the signed OpenID4VP request object (JAR) the
// wallet fetches by reference. While the ceremony is pending the JAR requests
// PID and PoA; after the document is prepared it is a Document-Retrieval JAR
// (ADR-12).
func (s *signatureManagementsrvc) SignatureRequestObject(ctx context.Context, p *signaturemanagement.SignatureRequestObjectPayload) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	ceremony, err := s.getCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", p.CeremonyID))
	}

	walletNonce := ""
	if p.WalletNonce != nil {
		walletNonce = strings.TrimSpace(*p.WalletNonce)
	}

	if ceremony.PreparedPDFSHA256 != nil && ceremony.RequestNonce != nil && ceremony.RequestExpiresAt != nil && ceremony.SignerDID != nil {
		published, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
		if err != nil {
			return nil, err
		}
		return s.buildDocumentRetrievalJAR(published, walletNonce)
	}

	pending, err := s.loadPendingCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}

	return s.buildIdentityPresentationJAR(ctx, pending, walletNonce)
}

// buildDocumentRetrievalJAR builds the published-ceremony JAR that asks the
// wallet to fetch and sign the prepared PDF AND the canonical JSON-LD payload
// (ADR-12: "document_digests is an array: the PDF and the JSON-LD are offered
// together, so one ceremony yields both a PAdES and a JAdES over the same
// content hash"). The payload's digest doubles as the nonce-binding and
// byte-pin anchor the callback checks the returned JAdES against (ADR-20).
func (s *signatureManagementsrvc) buildDocumentRetrievalJAR(ceremony *db.SignatureCeremony, walletNonce string) (io.ReadCloser, error) {
	if err := assertPreparedDocumentDigestConsistent(ceremony); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	pdfDigestBytes, decErr := hex.DecodeString(*ceremony.PreparedPDFSHA256)
	if decErr != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("decode prepared document digest: %w", decErr))
	}
	digests := []oid4vprequest.DocumentDigest{
		{Label: ceremony.FieldName, Hash: base64.StdEncoding.EncodeToString(pdfDigestBytes)},
	}
	locations := []oid4vprequest.DocumentLocation{
		{URI: s.signatureRequestURL(ceremony.ID, "document"), Method: oid4vprequest.DocumentLocationMethod{Type: "public"}},
	}
	if ceremony.PinnedPayloadSHA256 != nil && *ceremony.PinnedPayloadSHA256 != "" {
		payloadDigestBytes, decErr := hex.DecodeString(*ceremony.PinnedPayloadSHA256)
		if decErr != nil {
			return nil, signaturemanagement.MakeInternalError(fmt.Errorf("decode prepared payload digest: %w", decErr))
		}
		digests = append(digests, oid4vprequest.DocumentDigest{Label: ceremony.FieldName + "-payload", Hash: base64.StdEncoding.EncodeToString(payloadDigestBytes)})
		locations = append(locations, oid4vprequest.DocumentLocation{URI: s.signatureRequestURL(ceremony.ID, "payload"), Method: oid4vprequest.DocumentLocationMethod{Type: "public"}})
	}

	credentialType := "AES"
	if ceremony.CredentialType != nil && *ceremony.CredentialType != "" {
		credentialType = *ceremony.CredentialType
	}

	jwt, err := oid4vprequest.BuildDocumentRetrievalJWT(s.RequestSigner, oid4vprequest.DocRetrievalParams{
		ClientID:           s.OID4VPClientID,
		ResponseURI:        s.signatureRequestURL(ceremony.ID, "callback"),
		Nonce:              *ceremony.RequestNonce,
		ExpiresAt:          *ceremony.RequestExpiresAt,
		SignatureQualifier: signatureQualifierFor(credentialType),
		DocumentDigests:    digests,
		DocumentLocations:  locations,
		WalletNonce:        walletNonce,
	})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("build signing request object: %w", err))
	}
	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

// buildIdentityPresentationJAR builds the pending-ceremony JAR that asks the
// wallet for PID and PoA presentations.
func (s *signatureManagementsrvc) buildIdentityPresentationJAR(ctx context.Context, ceremony *db.SignatureCeremony, walletNonce string) (io.ReadCloser, error) {
	if s.RequestSigner == nil || s.OID4VPClientID == "" || s.PublicAPIBase == "" || s.PIDDCQLQuery == nil || s.DCQLQuery == nil {
		log.Printf(ctx, "SignatureRequestObject: OpenID4VP request signing is not configured")
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not build the authorization request"))
	}

	dcqlQuery, err := mergeSigningCeremonyDCQL(s.PIDDCQLQuery, s.DCQLQuery)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not build the authorization request"))
	}

	jwt, err := oid4vprequest.BuildJWT(s.RequestSigner, oid4vprequest.Params{
		ClientID:    s.OID4VPClientID,
		ResponseURI: s.signatureRequestURL(ceremony.ID, "callback"),
		State:       ceremony.ID,
		Nonce:       ceremony.Nonce,
		WalletNonce: walletNonce,
		ExpiresAt:   ceremony.ExpiresAt,
		DCQLQuery:   dcqlQuery,
	})
	if err != nil {
		log.Printf(ctx, "SignatureRequestObject: build JAR failed: %v", err)
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("could not build the authorization request"))
	}
	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

// SignatureRequestDocument serves the stored to-be-signed PDF the wallet fetches
// from the request object's document_locations.
func (s *signatureManagementsrvc) SignatureRequestDocument(ctx context.Context, p *signaturemanagement.SignatureRequestDocumentPayload) (io.ReadCloser, error) {
	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	if len(ceremony.PreparedPDF) == 0 {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no prepared document", p.CeremonyID))
	}
	// PreparedPDF and PreparedPDFSHA256 are written together in one
	// StorePreparedRequest call, but are read back here as two independently
	// persisted columns — a persistence-layer bug (wrong column read/scanned,
	// truncation, encoding mismatch) could silently desync them without ever
	// touching the in-memory write path a unit test would exercise. This is
	// exactly the document a real wallet is about to fetch and the digest the
	// JAR already told it to expect (buildDocumentRetrievalJAR); catch a
	// mismatch here, at the source, rather than as an opaque wallet-side
	// refusal with no attributable cause on our end.
	if err := assertPreparedDocumentDigestConsistent(ceremony); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	return io.NopCloser(bytes.NewReader(ceremony.PreparedPDF)), nil
}

// SignatureRequestPayload serves the pinned canonical JSON-LD contract payload
// the wallet fetches from the request object's SECOND document_locations
// entry, to produce the JAdES twin of the PAdES over the same content hash
// (ADR-12 SM-02/-11) — and, since it is served byte-for-byte identical to what
// was pinned at prepare, the same bytes the DCS's byte-pin check (ADR-20)
// compares the returned JAdES payload against.
func (s *signatureManagementsrvc) SignatureRequestPayload(ctx context.Context, p *signaturemanagement.SignatureRequestPayloadPayload) (io.ReadCloser, error) {
	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	if len(ceremony.PinnedPayload) == 0 {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no pinned payload", p.CeremonyID))
	}
	return io.NopCloser(bytes.NewReader(ceremony.PinnedPayload)), nil
}

// SignatureRequestCallback is the OpenID4VP response_uri for a ceremony. While
// pending it accepts a direct_post vp_token with PID and PoA; after publish it
// accepts the signed document (ADR-12).
func (s *signatureManagementsrvc) SignatureRequestCallback(ctx context.Context, p *signaturemanagement.SignatureRequestCallbackPayload, body io.ReadCloser) (res *signaturemanagement.SMSignatureRequestCallbackResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	form, err := parseDirectPostForm(body)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("parse direct_post body: %w", err))
	}
	if walletErr := strings.TrimSpace(form.Get("error")); walletErr != "" {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("wallet reported an error: %s", walletErr))
	}
	if state := strings.TrimSpace(form.Get("state")); state != "" && state != p.CeremonyID {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("callback state %q does not match ceremony %s", state, p.CeremonyID))
	}

	if vpToken := strings.TrimSpace(form.Get("vp_token")); vpToken != "" {
		return s.ceremonyPresentationDirectPost(ctx, p.CeremonyID, vpToken)
	}

	ceremony, err := s.loadPublishedCeremony(ctx, p.CeremonyID)
	if err != nil {
		return nil, err
	}
	// Fast-fail for the common case; NOT the consumption guard. The atomic
	// guard is inside Applier.SubmitSignature (ADR-20): its guarded UPDATE ...
	// WHERE consumed_at IS NULL and the finalize writes commit or roll back in
	// ONE transaction, so two concurrent callbacks can never both finalize
	// even though both may pass this early, non-atomic read.
	if ceremony.ConsumedAt != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s signing request has already been consumed", p.CeremonyID))
	}

	signedDocs := formList(form, "documentWithSignature")
	if len(signedDocs) == 0 {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("no documentWithSignature was posted"))
	}
	signedPDF, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(signedDocs[0]))
	if decErr != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("decode signed document: %w", decErr))
	}

	credentialType := "AES"
	if ceremony.CredentialType != nil && *ceremony.CredentialType != "" {
		credentialType = *ceremony.CredentialType
	}
	// documentWithSignature[] and signatureObject[] are two INDEPENDENT lists
	// for the same document index (CSC obtainSignedDoc's own shape,
	// ResponseDispatcher.Positive in the EUDI walletdriven-signer reference)
	// — not a positional split across documents. documentWithSignature is the
	// enveloped signature (the PAdES, embedded in the returned PDF);
	// signatureObject is a detached signature value, which is where the
	// detached-by-nature JAdES over the machine-readable JSON-LD rides.
	jades := ""
	if objects := formList(form, "signatureObject"); len(objects) > 0 {
		jades = strings.TrimSpace(objects[0])
	}
	appliedBy := ""
	if ceremony.PublishedBy != nil {
		appliedBy = *ceremony.PublishedBy
	}
	holderDID := ""
	if ceremony.PublishedHolderDID != nil {
		holderDID = *ceremony.PublishedHolderDID
	}
	var roles userrole.UserRoles
	if len(ceremony.PublishedRoles) > 0 {
		if err := json.Unmarshal(ceremony.PublishedRoles, &roles); err != nil {
			return nil, signaturemanagement.MakeInternalError(fmt.Errorf("decode publisher roles: %w", err))
		}
	}

	// The signature field is the participating party (org DID); the natural person
	// who signs is established separately, by the ceremony's verified PID. It is
	// NOT established by the signing certificate: AssertValidAES checks that the
	// signature is a valid AES and nothing more — no PID-to-certificate identifier
	// binding is standardised (see apply.go's SubmitSignature).
	applier := s.newApplier()
	if err := applier.SubmitSignature(ctx, command.SubmitSignatureCmd{
		ApplyCmd: command.ApplyCmd{
			DID:            ceremony.ContractDID,
			SignerDID:      *ceremony.SignerDID,
			FieldName:      ceremony.FieldName,
			CredentialType: credentialType,
			AppliedBy:      appliedBy,
			HolderDID:      holderDID,
			UserRoles:      roles,
			// The callback already resolved the EXACT ceremony from the URL's
			// ceremony_id (loadPublishedCeremony above) — pin submit to it
			// explicitly rather than falling back to resolveCeremony's "most
			// recent verified ceremony for this field" heuristic, which can
			// resolve a DIFFERENT ceremony than the one prepare pinned bytes
			// onto once more than one has been verified for the same field
			// (ADR-20 byte pinning).
			CeremonyID: ceremony.ID,
		},
		SignedPDF:      signedPDF,
		JAdESSignature: jades,
	}); err != nil {
		return nil, mapSignatureCommandError(err)
	}

	// SubmitSignature already marked the ceremony consumed atomically, in the
	// same transaction as finalize (ADR-20) — this is a read-only follow-up
	// for the response, not a second write.
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	processData, err := s.CRepo.ReadProcessDataByDID(ctx, tx, ceremony.ContractDID)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}

	did := ceremony.ContractDID
	return &signaturemanagement.SMSignatureRequestCallbackResponse{
		CeremonyID: ceremony.ID,
		Did:        &did,
		Status:     processData.State,
	}, nil
}

func (s *signatureManagementsrvc) ceremonyPresentationDirectPost(ctx context.Context, ceremonyID, vpToken string) (*signaturemanagement.SMSignatureRequestCallbackResponse, error) {
	ceremony, err := s.loadPendingCeremony(ctx, ceremonyID)
	if err != nil {
		return nil, err
	}

	presCtx := oid4vp.PresentationContext{
		Nonce:    ceremony.Nonce,
		ClientID: s.OID4VPClientID,
	}

	pidQueryIDs, err := credentialQueryIDsFromDCQL(s.PIDDCQLQuery)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("invalid pid dcql_query: %w", err))
	}
	poaQueryIDs, err := credentialQueryIDsFromDCQL(s.DCQLQuery)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(fmt.Errorf("invalid poa dcql_query: %w", err))
	}

	pidPresentation, err := extractSinglePresentation(vpToken, pidQueryIDs...)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("invalid vp_token: %w", err))
	}

	poaPresentation, err := extractSinglePresentation(vpToken, poaQueryIDs...)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("%w: no Power of Attorney credential was presented at signing", command.ErrPoAUnauthorized))
	}

	verifiedPoA, err := oid4vp.NewVerifier(s.Trust, oid4vp.PurposePeer).Verify(poaPresentation, presCtx)
	if err != nil {
		log.Printf(ctx, "SignatureRequestCallback: Verify PoA failed for ceremony %s: %v", ceremonyID, err)
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("vp verification failed: PoA: %w", err))
	}

	verifiedPID, err := oid4vp.NewVerifier(s.Trust, oid4vp.PurposePID).VerifyPID(pidPresentation, presCtx)
	if err != nil {
		log.Printf(ctx, "SignatureRequestCallback: VerifyPID failed for ceremony %s: %v", ceremonyID, err)
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("vp verification failed: PID: %w", err))
	}

	var pidClaims any
	if len(verifiedPID.RawClaims) > 0 {
		_ = json.Unmarshal(verifiedPID.RawClaims, &pidClaims)
	}

	// sdHash is extracted (not re-verified — VerifyPID above already did that,
	// against the ceremony's own nonce and the configured trust anchors) purely
	// as a record-keeping field for the KB-JWT credential-chain link (SM-26).
	presentation, err := sdjwt.ParsePresentation(pidPresentation)
	if err != nil {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("invalid vp_token: %w", err))
	}

	handler := command.PresentationHandler{DB: s.DB, CeremonyRepo: s.CeremonyRepo}
	verified, err := handler.CompletePresentation(ctx, command.PresentationCmd{
		CeremonyID:      ceremonyID,
		SignerDID:       verifiedPID.SubjectDID,
		SDHash:          presentation.SDHash,
		VpToken:         pidPresentation,
		PidClaims:       pidClaims,
		PoAOrganization: strings.TrimSpace(verifiedPoA.ParticipantDID),
		PoARoles:        verifiedPoA.Roles,
		PoAVpToken:      poaPresentation,
	})

	if err != nil {
		switch {
		case errors.Is(err, command.ErrPoAUnauthorized):
			return nil, signaturemanagement.MakeBadRequest(err)
		case errors.Is(err, command.ErrCeremonyExpired):
			return nil, signaturemanagement.MakeBadRequest(err)
		case errors.Is(err, command.ErrCeremonyNotFound):
			return nil, signaturemanagement.MakeNotFound(err)
		default:
			return nil, signaturemanagement.MakeInternalError(err)
		}
	}

	return &signaturemanagement.SMSignatureRequestCallbackResponse{
		CeremonyID: verified.ID,
		Status:     verified.Status,
	}, nil
}

// getCeremony reads a ceremony by id in a short read transaction.
func (s *signatureManagementsrvc) getCeremony(ctx context.Context, id string) (*db.SignatureCeremony, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	ceremony, err := s.CeremonyRepo.GetCeremonyByID(ctx, tx, id)
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	return ceremony, nil
}

// readContractDataByDID reads a contract's JSON-LD data in a short read
// transaction, so PublishSignatureRequest can default credential_type to the
// contract's OWN declared level requirement before Prepare's fail-fast check
// runs. Returns (nil, nil) rather than an error for a contract carrying no
// data yet — the caller falls back to "AES" in that case, and Prepare's own
// (authoritative) checks reject an actually-broken contract regardless.
func (s *signatureManagementsrvc) readContractDataByDID(ctx context.Context, did string) (*datatype.JSON, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	contract, err := s.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, nil
	}
	return contract.ContractData, nil
}

// loadPendingCeremony resolves a pending ceremony that has not yet been
// published as a document-retrieval signing request.
func (s *signatureManagementsrvc) loadPendingCeremony(ctx context.Context, id string) (*db.SignatureCeremony, error) {
	ceremony, err := s.getCeremony(ctx, id)
	if err != nil {
		return nil, err
	}

	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", id))
	}

	if ceremony.Status != db.CeremonyPending {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", id))
	}

	if !time.Now().UTC().Before(ceremony.ExpiresAt) {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony expired"))
	}

	return ceremony, nil
}

// loadPublishedCeremony resolves a ceremony that has a live published signing
// request (a prepared document, a fresh nonce, and an unexpired request), the
// precondition the object/document/callback endpoints share.
func (s *signatureManagementsrvc) loadPublishedCeremony(ctx context.Context, id string) (*db.SignatureCeremony, error) {
	ceremony, err := s.getCeremony(ctx, id)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s not found", id))
	}
	if ceremony.PreparedPDFSHA256 == nil || ceremony.RequestNonce == nil || ceremony.RequestExpiresAt == nil || ceremony.SignerDID == nil {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no published signing request", id))
	}
	if time.Now().UTC().After(*ceremony.RequestExpiresAt) {
		return nil, signaturemanagement.MakeBadRequest(fmt.Errorf("ceremony %s signing request has expired", id))
	}
	return ceremony, nil
}

// signatureRequestURL builds an absolute per-ceremony signing-request endpoint
// URL on the public API base.
func (s *signatureManagementsrvc) signatureRequestURL(ceremonyID, leaf string) string {
	return strings.TrimRight(s.PublicAPIBase, "/") + "/signature/request/" + url.PathEscape(ceremonyID) + "/" + leaf
}

// directPostMaxBytes bounds the wallet's direct_post body; a signed contract PDF
// with embedded evidence is a few MB.
const directPostMaxBytes = 64 << 20

// parseDirectPostForm reads and url-decodes the wallet's application/
// x-www-form-urlencoded direct_post body (the EUDI walletdriven-signer response).
func parseDirectPostForm(body io.ReadCloser) (url.Values, error) {
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, directPostMaxBytes))
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(string(raw))
}

// formList extracts a repeated form field the way the EUDI walletdriven-signer
// relying party does (retrieve_list_values_from_form_urlencoded): indexed keys
// (name[0], name[]) first, then repeated bare keys.
func formList(form url.Values, name string) []string {
	var indexed []string
	for key, values := range form {
		if strings.HasPrefix(key, name+"[") {
			indexed = append(indexed, values...)
		}
	}
	if len(indexed) > 0 {
		return indexed
	}
	return form[name]
}

// mergeSigningCeremonyDCQL folds the PID and PoA queries into the single DCQL
// query the pending ceremony's request object carries. The ceremony needs BOTH
// credentials, while each part may offer several alternative ways to satisfy it
// (the same credential under either SD-JWT VC format identifier), so the merged
// credential_sets is the cross product of the parts' own alternatives: every
// option names one acceptable query per part.
func mergeSigningCeremonyDCQL(parts ...any) (any, error) {
	var credentials []any
	combinations := [][]string{{}}

	for _, part := range parts {
		creds, options, err := credentialsAndOptionsFromDCQL(part)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, creds...)
		combinations = crossProduct(combinations, options)
	}

	if len(credentials) == 0 {
		return nil, fmt.Errorf("no credentials")
	}

	out := map[string]any{"credentials": credentials}

	if len(credentials) > 1 {
		options := make([]any, 0, len(combinations))
		for _, combination := range combinations {
			option := make([]any, len(combination))
			for i, id := range combination {
				option[i] = id
			}
			options = append(options, option)
		}
		out["credential_sets"] = []any{map[string]any{"options": options}}
	}

	return out, nil
}

// credentialsAndOptionsFromDCQL returns one query's credential entries and the
// alternative id-sets that satisfy it. DCQL without credential_sets means
// "satisfy every credential query", so that is the single option then.
func credentialsAndOptionsFromDCQL(dcqlQuery any) ([]any, [][]string, error) {
	query, ok := dcqlQuery.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("dcql query must be a JSON object")
	}

	rawCredentials, ok := query["credentials"]
	if !ok {
		return nil, nil, fmt.Errorf("missing credentials")
	}

	credentials, ok := rawCredentials.([]any)
	if !ok || len(credentials) == 0 {
		return nil, nil, fmt.Errorf("credentials must be a non-empty array")
	}

	ids := make([]string, 0, len(credentials))
	for _, cred := range credentials {
		entry, ok := cred.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}

	sets, ok := query["credential_sets"].([]any)
	if !ok || len(sets) == 0 {
		return credentials, [][]string{ids}, nil
	}

	options := [][]string{{}}
	for _, rawSet := range sets {
		set, ok := rawSet.(map[string]any)
		if !ok {
			continue
		}
		rawOptions, ok := set["options"].([]any)
		if !ok || len(rawOptions) == 0 {
			continue
		}
		setOptions := make([][]string, 0, len(rawOptions))
		for _, rawOption := range rawOptions {
			option, ok := rawOption.([]any)
			if !ok {
				continue
			}
			combination := make([]string, 0, len(option))
			for _, rawID := range option {
				if id, ok := rawID.(string); ok && strings.TrimSpace(id) != "" {
					combination = append(combination, strings.TrimSpace(id))
				}
			}
			setOptions = append(setOptions, combination)
		}
		options = crossProduct(options, setOptions)
	}

	return credentials, options, nil
}

func crossProduct(left, right [][]string) [][]string {
	if len(right) == 0 {
		return left
	}

	out := make([][]string, 0, len(left)*len(right))
	for _, l := range left {
		for _, r := range right {
			combined := make([]string, 0, len(l)+len(r))
			combined = append(combined, l...)
			combined = append(combined, r...)
			out = append(out, combined)
		}
	}

	return out
}
