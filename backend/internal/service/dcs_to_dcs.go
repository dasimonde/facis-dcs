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
	"time"

	"goa.design/clue/log"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
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
	TrustGate   trustgate.TrustGate
	PoAGate     trustgate.CounterpartyPoAGate
	Shredder    trustgate.ScopeShredder
	Parties     trustgate.ContractParties
	auth.JWTAuthenticator
}

func NewDcsToDcs(db *sqlx.DB, jwtAuth auth.JWTAuthenticator,
	cRepo db.ContractRepo, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo,
	ntRepo db.NegotiationTaskRepo, nRepo db.NegotiationRepo, ctRepo db.ContractTemplateRepo, syncRepo db2.SyncRepository,
	trustPool *identity.EUTrustPool,
	didDocument identity.DIDDocument, artifacts *artifactstore.Store, pdfCore *pdfcore.Client, trustGate trustgate.TrustGate,
	poaGate trustgate.CounterpartyPoAGate, shredder trustgate.ScopeShredder) dcstodcs.Service {

	return &dcsToDcssrvc{
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
		TrustGate:        trustGate,
		PoAGate:          poaGate,
		Shredder:         shredder,
		Parties:          &trustgate.DBContractParties{DB: db, CRepo: cRepo},
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
	remoteDIDDocument, err := fetchPeerDIDDocument(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := remoteDIDDocument.VerifyPeerChallenge(s.TrustPool, []byte(req.SecretValue), req.SecretHash); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if identity.SameDIDWeb(req.FromPeerDid, localPeer) {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("shipping a contract PDF to the same peer is not allowed"))
	}

	// Federation trust gate (ADR-19): the peer's self-signed agreement
	// credential must verify against its own did.json's dedicated VC key and
	// name this instance's own embedded federation rules hash (layer 3a), and
	// this instance's own local policy endpoint must allow the interaction
	// (layer 3b). Any rejection is recorded as an incident in the audit trail.
	if err := s.TrustGate.Check(ctx, req.FromPeerDid, trustgate.Inbound, req.ContractIri, ""); err != nil {
		var gateErr *trustgate.GateError
		if errors.As(err, &gateErr) {
			if incidentErr := trustgate.RecordDenialIncident(ctx, s.DB, req.ContractIri, trustgate.Inbound, gateErr); incidentErr != nil {
				log.Printf(ctx, "could not record trust gate denial incident for %s: %v", req.ContractIri, incidentErr)
			}
		}
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_pdf rejected: peer %s does not pass the federation trust gate: %w", req.FromPeerDid, err))
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

	receiver := command.PeerPdfReceiver{DB: s.DB, CRepo: s.CRepo, RTRepo: s.RTRepo, ATRepo: s.ATRepo, NTRepo: s.NTRepo, Artifacts: s.Artifacts}
	if err := receiver.Handle(ctx, command.PeerPdfReceiveCmd{
		ContractIRI:  req.ContractIri,
		Counterparty: req.FromPeerDid,
		LocalPeer:    localPeer,
		Payload:      payload,
		Pdf:          req.Pdf,
		AdoptRevoked: adoptRevoked,
	}); err != nil {
		if artifactstore.IsShredded(err) {
			return nil, contractworkflowengine.MakeBadRequest(
				fmt.Errorf("post_pdf rejected: contract %s was erased on this instance (key shredded): %w", req.ContractIri, err))
		}
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	if syncSignature != nil {
		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := s.SRepo.UpsertSyncSignature(ctx, tx, *syncSignature); err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		if err := tx.Commit(); err != nil {
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

// Erase shreds this instance's wrapped CEKs for a contract on request of the
// authenticated counterparty (DCS-NFR-COMP-03, DCS-NFR-SEC-13): the peer
// completed an archive deletion and erasure of a federated contract requires
// key destruction on BOTH instances. The requester passes the same did:web
// challenge-response as post_pdf and must be a party of the contract; the
// shred marks every CEK record of the contract scope destroyed and leaves a
// KEY_SHREDDED audit event naming the peer as actor. Idempotent: a repeated
// request against an already-shredded contract just confirms again.
func (s *dcsToDcssrvc) Erase(ctx context.Context, req *dcstodcs.DCSToDCSContractEraseRequest) (res *dcstodcs.DCSToDCSContractEraseResponse, err error) {
	remoteDIDDocument, err := fetchPeerDIDDocument(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := remoteDIDDocument.VerifyPeerChallenge(s.TrustPool, []byte(req.SecretValue), req.SecretHash); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(err)
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if identity.SameDIDWeb(req.FromPeerDid, localPeer) {
		return nil, contractworkflowengine.MakeBadRequest(errors.New("requesting a contract erasure from the same peer is not allowed"))
	}

	if err := eraseForPeer(ctx, s.Shredder, s.Parties, req.FromPeerDid, req.ContractIri); err != nil {
		return nil, err
	}
	return &dcstodcs.DCSToDCSContractEraseResponse{FromPeerDid: localPeer}, nil
}

// eraseForPeer authorizes and executes a peer-requested shred: only a party
// of the contract may have its keys destroyed on the counterparty's request.
func eraseForPeer(ctx context.Context, shredder trustgate.ScopeShredder, parties trustgate.ContractParties, peerDID, contractIRI string) error {
	contractParties, err := parties.Parties(ctx, contractIRI)
	if err != nil {
		return contractworkflowengine.MakeBadRequest(
			fmt.Errorf("erase rejected: could not resolve parties of contract %s: %w", contractIRI, err))
	}
	isParty := false
	for _, party := range contractParties {
		if party == peerDID {
			isParty = true
			break
		}
	}
	if !isParty {
		return contractworkflowengine.MakeBadRequest(
			fmt.Errorf("erase rejected: peer %s is not a party of contract %s", peerDID, contractIRI))
	}

	if _, err := shredder.Shred(ctx, contractIRI, peerDID, fmt.Sprintf("erasure requested by peer %s", peerDID)); err != nil {
		return contractworkflowengine.MakeInternalError(err)
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
func (s *dcsToDcssrvc) GetProvenance(ctx context.Context, p *dcstodcs.GetProvenancePayload) (res *dcstodcs.DCSToDCSSyncProvenanceResponse, err error) {
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
