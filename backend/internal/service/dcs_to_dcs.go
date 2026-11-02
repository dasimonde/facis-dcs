package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"goa.design/clue/log"
	goa "goa.design/goa/v3/pkg"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/base/tsa"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"

	trustgate "digital-contracting-service/internal/dcstodcs"

	"digital-contracting-service/internal/contractworkflowengine/command"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/semantichub"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/contractworkflowengine/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/auth"

	"github.com/jmoiron/sqlx"
)

type dcsToDcssrvc struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	NRepo       db.NegotiationRepo
	CTRepo      db.ContractTemplateRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
	TrustPool   *identity.EUTrustPool
	Artifacts   *artifactstore.Store
	PDFCore     *pdfcore.Client
	TrustGate   peerGate
	Nonces      trustgate.PeerNonces
	PoAGate     trustgate.CounterpartyPoAGate
	Shredder    trustgate.ScopeShredder
	Erasure     trustgate.PeerErasure
	// ArchiveNotary and ArchiveTSA notarize and timestamp the execution proof
	// written when the last counterparty signature arrives (DCS-FR-SM-10).
	ArchiveNotary command.ArchiveNotary
	ArchiveTSA    command.ArchiveTimestampIssuer
	auth.JWTAuthenticator
}

// peerGate is the ADR-19 federation trust gate of the inbound peer requests.
type peerGate interface {
	Check(ctx context.Context, peerDID string, direction trustgate.Direction, contractDID, targetState string) error
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, syncRepo db2.SyncRepository,
	trustPool *identity.EUTrustPool,
	didDocument identity.DIDDocument, artifacts *artifactstore.Store, pdfCore *pdfcore.Client, trustGate trustgate.TrustGate,
	poaGate trustgate.CounterpartyPoAGate, shredder trustgate.ScopeShredder,
	archiveNotary command.ArchiveNotary, archiveTSA *tsa.APIClient) dcstodcs.Service {

	return &dcsToDcssrvc{
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		RTRepo:           rtRepo,
		ATRepo:           atRepo,
		NTRepo:           ntRepo,
		NRepo:            nRepo,
		CTRepo:           ctRepo,
		SRepo:            syncRepo,
		DIDDocument:      didDocument,
		TrustPool:        trustPool,
		Artifacts:        artifacts,
		PDFCore:          pdfCore,
		TrustGate:        &trustGate,
		Nonces:           trustgate.DBPeerNonces{DB: db},
		PoAGate:          poaGate,
		Shredder:         shredder,
		Erasure:          trustgate.DBPeerErasure{DB: db, CRepo: cRepo},
	}
}

// fetchPeerDIDDocument resolves the requesting peer's did:web document;
// injectable for tests.
var fetchPeerDIDDocument = identity.FetchDIDDocument

// assertionKeyOf resolves the assertion key of the instance that OWNS a signed
// field, so its embedded signing summary is checked against ITS key rather than
// the shipper's. A shipped PDF carries one attachment per signing event and a
// countersigned one therefore carries evidence issued by more than one instance.
//
// The two documents already in hand answer for themselves; a field naming a
// third did:web is resolved the same way the shipping peer's was.
func (s *dcsToDcssrvc) assertionKeyOf(ownerDID, localPeer, peerDID string, peerDocument *identity.DIDDocument, methodID string) (*ecdsa.PublicKey, error) {
	switch {
	case identity.SameDIDWeb(ownerDID, localPeer):
		return s.DIDDocument.AssertionKey(methodID)
	case identity.SameDIDWeb(ownerDID, peerDID):
		return peerDocument.AssertionKey(methodID)
	}
	document, err := fetchPeerDIDDocument(ownerDID)
	if err != nil {
		return nil, fmt.Errorf("resolve the did document of %s, which the embedded signing evidence names as the signing instance: %w", ownerDID, err)
	}
	return document.AssertionKey(methodID)
}

// PostPdf receives a contract PDF a counterparty shipped (ADR-13). It
// authenticates the peer (the same layers post_sync applied), asks pdf-core to
// extract the embedded JSON-LD, and upserts this instance's own local copy of
// the contract. No tasks cross the boundary — each DCS runs its own workflow.
func (s *dcsToDcssrvc) PostPdf(ctx context.Context, req *dcstodcs.DCSToDCSContractPdfRequest) (res *dcstodcs.DCSToDCSContractPdfResponse, err error) {
	defer func() { s.auditReceivedExchange(ctx, "post_pdf", req.ContractIri, req.FromPeerDid, err) }()
	remoteDIDDocument, localPeer, err := s.admitPeer(ctx, trustgate.OpPostPdf, req.FromPeerDid, req.ContractIri, req.Pdf, req.SecretValue, req.SecretHash)
	if err != nil {
		return nil, err
	}

	// Legal gate: the received PDF's human-readable page content MUST be the
	// deterministic re-render of its own embedded machine-readable payload, or
	// the two forms of the contract have diverged and we refuse it. pdf-core
	// /verify/content compares only the page content streams, so the C2PA,
	// signature and amendment layers a peer legitimately appended do not trip
	// it; genuine tampering does.
	contentMatch, mismatchDetail, verr := s.PDFCore.VerifyContent(ctx, req.Pdf)
	if verr != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: could not content-verify received PDF: %w", verr))
	}
	if !contentMatch {
		// Diagnostic (rejection path only; the fatal gate is unchanged): surface
		// WHICH page diverged + a snippet of both renders, plus the embedded
		// payload's len+hash, so the exact human↔machine divergence is visible in
		// the peer's log.
		embedded, _ := s.PDFCore.ExtractPayload(ctx, req.Pdf)
		esum := sha256.Sum256(embedded)
		log.Printf(ctx, "post_pdf VerifyContent mismatch for %s: %s | embedded payload len=%d sha256=%s",
			req.ContractIri, mismatchDetail, len(embedded), hex.EncodeToString(esum[:8]))
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf(
			"post_pdf rejected: received PDF's human-readable page content does not match its embedded machine-readable payload: %s", mismatchDetail))
	}

	payload, err := s.PDFCore.ExtractPayload(ctx, req.Pdf)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: could not extract contract payload from PDF: %w", err))
	}

	// A ship accompanied by a JAdES is a signature (acceptance, DCS-FR-SM-02):
	// the JAdES must verify against the SENDER's did:web key and its payload
	// must bind exactly the contract this PDF carries. The challenge-response
	// secret above only authenticates the session — this binds the contract
	// CONTENT to the sender's key and leaves an independently verifiable
	// artifact behind (served by get_provenance).
	var syncSignature *db2.SyncSignature
	if req.JadesSignature != nil && *req.JadesSignature != "" {
		verified, err := verifyShippedJades(*req.JadesSignature, req.ContractIri, req.FromPeerDid, payload, remoteDIDDocument)
		if err != nil {
			return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_pdf rejected: %w", err))
		}
		syncSignature = verified
	}

	// Counterparty Power of Attorney (ADR-31, ADR-35, UC-14): every signing party
	// embeds its own summary credential and Power of Attorney into the PDF before
	// applying its signature, so the shipped artifact carries the authorization
	// behind each signature it holds — including this instance's own once the
	// contract has been countersigned and shipped back. Each is verified here:
	// issuer trusted for `peer` and entitled to the organization, not revoked,
	// held by the signatory the summary attests. Present-but-unverifiable
	// evidence refuses the exchange like any other trust-gate denial; absent
	// evidence does not, so a peer whose PDF carries none still federates and the
	// compliance viewer keeps reporting a party that signed without one.
	embeddedEvidence, err := s.PDFCore.ExtractEvidence(ctx, req.Pdf)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: could not extract the signing evidence embedded in the received PDF: %w", err))
	}
	shipped := trustgate.ShippedSignatures{
		ResolveKey: func(ownerDID, methodID string) (*ecdsa.PublicKey, error) {
			return s.assertionKeyOf(ownerDID, localPeer, req.FromPeerDid, remoteDIDDocument, methodID)
		},
		VerifyVC: provenance.VerifyDataIntegrityProof,
	}
	if err := s.PoAGate.Check(req.FromPeerDid, localPeer, req.ContractIri, shipped, embeddedEvidence); err != nil {
		var gateErr *trustgate.GateError
		if errors.As(err, &gateErr) {
			if incidentErr := trustgate.RecordDenialIncident(ctx, s.DB, req.ContractIri, trustgate.Inbound, gateErr); incidentErr != nil {
				log.Printf(ctx, "could not record trust gate denial incident for %s: %v", req.ContractIri, incidentErr)
			}
		}
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: peer %s shipped a Power of Attorney that does not verify: %w", req.FromPeerDid, err))
	}
	peerSignatures, err := trustgate.PeerSignatures(localPeer, embeddedEvidence)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_pdf rejected: %w", err))
	}
	if syncSignature != nil && len(peerSignatures) > 0 {
		evidence, err := json.Marshal(peerSignatures)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		revalidatedAt := time.Now().UTC()
		syncSignature.PoAEvidence = evidence
		syncSignature.PoARevalidatedAt = &revalidatedAt
	}

	// Semantic bundle (ADR-8): the shipped contract's dcs:effectiveShapes pin was
	// written from the SENDER's hub, so the shape libraries it names travel with
	// the ship. Settled here as a pure read — a pin that resolves neither from
	// this hub nor from the ship refuses the exchange rather than storing a copy
	// no workflow transition can evaluate — and written only once the rest of
	// the exchange has been accepted, so a refusal further down leaves nothing
	// behind.
	pinnedShapes := semantichub.DBPinnedShapes{DB: s.DB}
	installShapes, err := trustgate.PlanPinnedShapes(ctx, pinnedShapes, req.FromPeerDid, payload, req.PinnedShapes)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: the shapes contract %s is pinned to cannot be assembled on this instance: %w", req.ContractIri, err))
	}

	// The sender's declared contract state is informational except for
	// REVOKED: the authenticated counterparty revoking its own signature
	// voids the agreement, so the receiver must adopt it (DCS-NFR-BR-06) —
	// no other peer-declared state ever overrides the local workflow.
	adoptRevoked := req.ContractState != nil && *req.ContractState == contractstate.Revoked.String()

	// Adopt the shipped contract CEK (DCS-NFR-SEC-14): unwrap it with the own
	// HSM, re-wrap it to the own keyAgreement key, persist — then the PDF below
	// is stored under exactly this CEK. Runs only after every rejection gate,
	// so no key material is persisted for a refused ship. Adoption is
	// idempotent (an existing live CEK wins) and a missing wrapped_cek falls
	// back to creating an own CEK on store.
	if req.WrappedCek != nil {
		wrapped, err := trustgate.EnvelopeWrappedCEK(req.WrappedCek)
		if err != nil {
			return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_pdf rejected: %w", err))
		}
		if err := s.Artifacts.AdoptPeerCEK(ctx, artifactstore.ContractScope(req.ContractIri), wrapped); err != nil {
			if artifactstore.IsShredded(err) {
				return nil, contractworkflowengine.MakeBadRequest(
					fmt.Errorf("post_pdf rejected: contract %s was erased on this instance (key shredded): %w", req.ContractIri, err))
			}
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	}

	receiver := command.PeerPdfReceiver{DB: s.DB, CRepo: s.CRepo, NRepo: s.NRepo, RTRepo: s.RTRepo, ATRepo: s.ATRepo, NTRepo: s.NTRepo, Artifacts: s.Artifacts}
	if err := receiver.Handle(ctx, command.PeerPdfReceiveCmd{
		ContractIRI:  req.ContractIri,
		Counterparty: req.FromPeerDid,
		LocalPeer:    localPeer,
		Payload:      payload,
		Pdf:          req.Pdf,
		AdoptRevoked: adoptRevoked,
	}); err != nil {
		return nil, receiverError(req.ContractIri, err)
	}

	if syncSignature != nil {
		if err := s.recordSyncSignature(ctx, *syncSignature, localPeer, req.Pdf); err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	}

	// Last write of the exchange: nothing after it can refuse the ship and
	// strand the imported rows. A failure here errors the ship, and the peer's
	// retry carries the identical bundle, so the install is reached again.
	if err := trustgate.InstallPinnedShapes(ctx, pinnedShapes, installShapes); err != nil {
		return nil, contractworkflowengine.MakeInternalError(
			fmt.Errorf("install the shapes contract %s is pinned to: %w", req.ContractIri, err))
	}

	return &dcstodcs.DCSToDCSContractPdfResponse{FromPeerDid: localPeer}, nil
}

// receiverError maps an error of the local receiver to the post_pdf error the
// peer gets. A signing conflict is a conflict, so the peer does not ship the
// same copy again (ADR-38 §2).
func receiverError(contractIRI string, err error) error {
	switch {
	case artifactstore.IsShredded(err):
		return contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: contract %s was erased on this instance (key shredded): %w", contractIRI, err))
	case errors.Is(err, command.ErrSigningConflict):
		return dcstodcs.MakeConflict(fmt.Errorf("post_pdf rejected: %w", err))
	case errors.Is(err, command.ErrClosedCopy):
		return dcstodcs.MakeGone(fmt.Errorf("post_pdf rejected: %w", err))
	case errors.Is(err, command.ErrShippedDisagreesWithLocalRound):
		return contractworkflowengine.MakeBadRequest(fmt.Errorf("post_pdf rejected: %w", err))
	}
	return contractworkflowengine.MakeInternalError(err)
}

// recordSyncSignature stores the verified signature a peer shipped. This
// instance archives the contract when its own signatures are complete, so a
// counterparty signature that arrives later goes into that archive entry too.
func (s *dcsToDcssrvc) recordSyncSignature(ctx context.Context, signature db2.SyncSignature, localPeer string, signedPDF []byte) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.SRepo.UpsertSyncSignature(ctx, tx, signature); err != nil {
		return err
	}
	if len(signature.PoAEvidence) > 0 {
		if _, err := s.CRepo.AppendArchivePeerSignatures(ctx, tx, signature.DID, signature.PoAEvidence); err != nil {
			return fmt.Errorf("add the peer signatures to the archive entry of %s: %w", signature.DID, err)
		}
	}
	// The last counterparty signature completes the contract after this
	// instance archived it: the proof of execution covers it (DCS-FR-SM-10).
	if _, err := s.executionProofs().Record(ctx, tx, command.ExecutionProofInput{
		DID: signature.DID, LocalPeer: localPeer, PeerDID: signature.FromPeerDID, PeerJAdES: signature.JadesSignature,
		SignedPDF: signedPDF, StoredBy: signature.FromPeerDID,
	}); err != nil {
		return fmt.Errorf("record the execution proof of %s: %w", signature.DID, err)
	}
	return tx.Commit()
}

func (s *dcsToDcssrvc) executionProofs() command.ExecutionProofRecorder {
	recorder := command.ExecutionProofRecorder{CRepo: s.CRepo, Notary: s.ArchiveNotary, TSA: s.ArchiveTSA}
	if s.Artifacts != nil {
		recorder.Storer = s.Artifacts
	}
	return recorder
}

// Erase shreds this instance's wrapped CEKs for a contract on request of the
// authenticated counterparty (DCS-NFR-COMP-03, DCS-NFR-SEC-13): the peer
// completed an archive deletion and erasure of a federated contract requires
// key destruction on BOTH instances. The requester passes the same did:web
// challenge-response as post_pdf and must be a party of the contract; the
// shred marks every CEK record of the contract scope destroyed and leaves a
// KEY_SHREDDED audit event naming the peer as actor. Idempotent: a repeated
// request against an already-shredded contract just confirms again.
func (s *dcsToDcssrvc) Erase(ctx context.Context, req *dcstodcs.DCSToDCSContractEraseRequest) (res *dcstodcs.DCSToDCSContractEraseResponse, err error) {
	defer func() { s.auditReceivedExchange(ctx, "erase", req.ContractIri, req.FromPeerDid, err) }()
	_, localPeer, err := s.admitPeer(ctx, trustgate.OpErase, req.FromPeerDid, req.ContractIri, nil, req.SecretValue, req.SecretHash)
	if err != nil {
		return nil, err
	}
	if err := eraseForPeer(ctx, s.Shredder, s.Erasure, req.FromPeerDid, req.ContractIri); err != nil {
		return nil, err
	}
	return &dcstodcs.DCSToDCSContractEraseResponse{FromPeerDid: localPeer}, nil
}

// admitPeer authenticates an inbound peer request (ADR-13) and runs the
// federation trust gate (ADR-19) for it. Every peer endpoint calls it before
// it reads or writes anything else. It returns the peer's DID document and the
// DID of this instance.
func (s *dcsToDcssrvc) admitPeer(ctx context.Context, op trustgate.PeerOp, fromDID, contractIRI string, body []byte, secretValue string, secretHash []byte) (*identity.DIDDocument, string, error) {
	peerDocument, err := fetchPeerDIDDocument(fromDID)
	if err != nil {
		return nil, "", contractworkflowengine.MakeInternalError(err)
	}
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, "", contractworkflowengine.MakeInternalError(err)
	}
	if identity.SameDIDWeb(fromDID, localPeer) {
		return nil, "", contractworkflowengine.MakeBadRequest(
			fmt.Errorf("%s rejected: a request of this instance to itself is no peer exchange", op.Name))
	}
	if err := trustgate.VerifyPeerRequest(ctx, s.Nonces, peerDocument, s.TrustPool, localPeer, trustgate.PeerRequest{
		Op: op, FromDID: fromDID, ContractIRI: contractIRI, Body: body, SecretValue: secretValue, SecretHash: secretHash,
	}, time.Now()); err != nil {
		if !errors.Is(err, trustgate.ErrPeerChallengeRefused) {
			return nil, "", contractworkflowengine.MakeInternalError(err)
		}
		log.Printf(ctx, "%s from %s for %s refused: %v", op.Name, fromDID, contractIRI, err)
		return nil, "", contractworkflowengine.MakeBadRequest(
			fmt.Errorf("%s rejected: peer %s did not authenticate: %w", op.Name, fromDID, err))
	}
	if err := s.TrustGate.Check(ctx, fromDID, trustgate.Inbound, contractIRI, ""); err != nil {
		var gateErr *trustgate.GateError
		if errors.As(err, &gateErr) {
			if incidentErr := trustgate.RecordDenialIncident(ctx, s.DB, contractIRI, trustgate.Inbound, gateErr); incidentErr != nil {
				log.Printf(ctx, "could not record trust gate denial incident for %s: %v", contractIRI, incidentErr)
			}
		}
		return nil, "", contractworkflowengine.MakeBadRequest(
			fmt.Errorf("%s rejected: peer %s does not pass the federation trust gate: %w", op.Name, fromDID, err))
	}
	return peerDocument, localPeer, nil
}

// auditReceivedExchange records a completed inbound peer exchange in the
// contract's audit trail (DCS-NFR-BR-08). Refused requests are not recorded
// here: an unauthenticated caller must not be able to write the audit trail.
// A failure to record is logged; the exchange itself already completed.
func (s *dcsToDcssrvc) auditReceivedExchange(ctx context.Context, exchange, contractIRI, peerDID string, exchangeErr error) {
	if exchangeErr != nil {
		return
	}
	if err := trustgate.RecordPeerExchange(context.WithoutCancel(ctx), s.DB, contractIRI, exchange, trustgate.ExchangeReceived, peerDID); err != nil {
		log.Printf(ctx, "could not audit received %s for %s from %s: %v", exchange, contractIRI, peerDID, err)
	}
}

// eraseForPeer authorizes and executes a peer-requested shred: only a party
// of the contract may have its keys destroyed on the counterparty's request,
// and only when the local copy is no longer in force and no retention period
// of its archive entries runs (DCS-FR-CWE-20, DCS-FR-CSA-17).
func eraseForPeer(ctx context.Context, shredder trustgate.ScopeShredder, erasure trustgate.PeerErasure, peerDID, contractIRI string) error {
	if err := erasure.AuthorizeErase(ctx, contractIRI, peerDID); err != nil {
		switch {
		case errors.Is(err, trustgate.ErrNoSharedCopy):
			return dcstodcs.MakeNotFound(fmt.Errorf("erase rejected: %w", err))
		case errors.Is(err, trustgate.ErrNotErasable):
			return contractworkflowengine.MakeBadRequest(fmt.Errorf("erase rejected: %w", err))
		}
		return contractworkflowengine.MakeInternalError(err)
	}

	if _, err := shredder.Shred(ctx, contractIRI, peerDID, fmt.Sprintf("erasure requested by peer %s", peerDID)); err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	return nil
}

// Terminate applies a counterparty's termination notice to the local copy
// (DCS-NFR-BR-06).
func (s *dcsToDcssrvc) Terminate(ctx context.Context, req *dcstodcs.DCSToDCSContractTerminateRequest) (res *dcstodcs.DCSToDCSContractTerminateResponse, err error) {
	defer func() { s.auditReceivedExchange(ctx, "terminate", req.ContractIri, req.FromPeerDid, err) }()
	_, localPeer, err := s.admitPeer(ctx, trustgate.OpTerminate, req.FromPeerDid, req.ContractIri, trustgate.TerminationNoticeBody(req.Reason), req.SecretValue, req.SecretHash)
	if err != nil {
		return nil, err
	}
	if err := terminateForPeer(ctx, s.DB, s.CRepo, s.SRepo, s.DIDDocument, req); err != nil {
		return nil, err
	}
	return &dcstodcs.DCSToDCSContractTerminateResponse{FromPeerDid: localPeer}, nil
}

// terminateForPeer authorizes and executes a peer's termination notice: only a
// party of the contract may terminate it, and a contract already terminated
// confirms again.
func terminateForPeer(ctx context.Context, database *sqlx.DB, cRepo db.ContractRepo, sRepo db2.SyncRepository, didDocument identity.DIDDocument, req *dcstodcs.DCSToDCSContractTerminateRequest) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	exists, err := cRepo.ExistsByDID(ctx, tx, req.ContractIri)
	var contract *db.Contract
	if err == nil && exists {
		contract, err = cRepo.ReadDataByDID(ctx, tx, req.ContractIri)
	}
	_ = tx.Rollback()
	if err != nil {
		return contractworkflowengine.MakeInternalError(fmt.Errorf("termination: could not read contract %s: %w", req.ContractIri, err))
	}
	if contract == nil || contract.Responsible == nil || !slices.Contains(contract.Responsible.GetParties(), req.FromPeerDid) {
		return dcstodcs.MakeNotFound(fmt.Errorf("termination rejected: this instance holds no contract %s shared with peer %s", req.ContractIri, req.FromPeerDid))
	}
	// A copy that is no longer in force has nothing left to terminate: the
	// notice is answered as delivered, so the sender does not retry it.
	if contractstate.ContractState(contract.State).Erasable() {
		return nil
	}

	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}
	handler := command.Terminator{DB: database, CRepo: cRepo, SRepo: sRepo, DIDDocument: didDocument}
	if err := handler.Handle(ctx, command.TerminateCmd{
		DID:          req.ContractIri,
		TerminatedBy: req.FromPeerDid,
		Reason:       reason,
		UpdatedAt:    contract.UpdatedAt,
		CauserDID:    req.FromPeerDid,
	}); err != nil {
		return mapContractCommandError(err)
	}
	return nil
}

// verifyShippedJades verifies a signature ship's JAdES (DCS-FR-SM-02): the
// compact JWS must verify against its own x5c chain, the x5c leaf key must be
// the sender's published did:web key, and the signed payload must be exactly
// the JCS canonicalization of the contract the shipped PDF itself carries —
// same recipe the pre-ADR-13 post_sync handler applied. Returns the
// provenance artifact to persist for get_provenance.
func verifyShippedJades(jadesSignature, contractIRI, fromPeerDID string, pdfPayload []byte, remoteDIDDocument *identity.DIDDocument) (*db2.SyncSignature, error) {
	jadesPayload, leafKey, err := jades.Verify(jadesSignature)
	if err != nil {
		return nil, err
	}
	// The JAdES header names no key, only its x5c chain, so the leaf key itself
	// has to be one the peer publishes as able to make assertions — signing a
	// contract is one. A key merely present in the document is not: the same
	// document publishes the peer's key-agreement key.
	if !remoteDIDDocument.PublishesKeyFor(identity.PurposeAssertion, leafKey) {
		return nil, fmt.Errorf("JAdES x5c leaf key is not published by peer %s as an %s key",
			fromPeerDID, identity.PurposeAssertion)
	}

	// The payload is self-describing ({dcs:contractDid, dcs:contractVersion,
	// dcs:contractDocument}) — read the claimed identity out of the VERIFIED
	// bytes, then require it to bind this ship's contract and document.
	var claimed struct {
		ContractDid     string `json:"dcs:contractDid"`
		ContractVersion int    `json:"dcs:contractVersion"`
	}
	if err := json.Unmarshal(jadesPayload, &claimed); err != nil {
		return nil, fmt.Errorf("could not decode JAdES payload: %w", err)
	}
	if claimed.ContractDid != contractIRI {
		return nil, fmt.Errorf("JAdES payload binds contract %s, not the shipped contract %s", claimed.ContractDid, contractIRI)
	}
	expectedPayload, err := jades.BuildContractPayload(claimed.ContractDid, claimed.ContractVersion, pdfPayload)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(jadesPayload, expectedPayload) {
		return nil, fmt.Errorf("JAdES payload does not match the contract document embedded in the shipped PDF for %s", contractIRI)
	}

	return &db2.SyncSignature{
		DID:             contractIRI,
		ContractVersion: claimed.ContractVersion,
		FromPeerDID:     fromPeerDID,
		JadesSignature:  jadesSignature,
	}, nil
}

// GetProvenance returns the stored JAdES provenance artifact for a contract
// this instance received from a peer (DCS-FR-SM-02).
//
// The caller must be allowed to read the contract (DCS-FR-CWE-07). A contract
// the caller may not read answers not_found, like one without provenance.
func (s *dcsToDcssrvc) GetProvenance(ctx context.Context, p *dcstodcs.GetProvenancePayload) (res *dcstodcs.DCSToDCSSyncProvenanceResponse, err error) {
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		var serviceErr *goa.ServiceError
		if errors.As(err, &serviceErr) && (serviceErr.Name == "forbidden" || serviceErr.Name == "not_found") {
			return nil, dcstodcs.MakeNotFound(fmt.Errorf("no sync provenance stored for contract %s", p.Did))
		}
		return nil, err
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	sig, err := s.SRepo.GetSyncSignature(ctx, tx, p.Did)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if sig == nil {
		return nil, dcstodcs.MakeNotFound(fmt.Errorf("no sync provenance stored for contract %s", p.Did))
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSSyncProvenanceResponse{
		Did:             sig.DID,
		ContractVersion: sig.ContractVersion,
		FromPeerDid:     sig.FromPeerDID,
		JadesSignature:  sig.JadesSignature,
		ReceivedAt:      sig.ReceivedAt.UTC().Format(time.RFC3339),
	}, nil
}
