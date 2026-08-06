package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/base/validation"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cweevent "digital-contracting-service/internal/contractworkflowengine/event"
	dcsdb "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	event2 "digital-contracting-service/internal/signingmanagement/event"

	"github.com/digitorus/pkcs7"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ErrCeremonyRequired is the typed precondition failure returned when a
// signature is applied for a signer/contract that has no completed PID
// presentation ceremony (DCS-FR-SM-16, FR-SM-25, UC-04-02).
var ErrCeremonyRequired = errors.New("a completed PID presentation ceremony is required before signing")

// ErrCeremoniesIncomplete is returned by the multi-signer flow's
// all-ceremonies-before-first-signature gate (DCS-FR-SM-07/-17): every
// declared signature field needs a verified ceremony before the FIRST
// signature is applied, because every signer's evidence must be embedded
// into the PDF before any PAdES signature freezes it (embedding an
// attachment after a signature trips standards-compliant diff analysis).
var ErrCeremoniesIncomplete = errors.New("all declared signature fields need a completed PID presentation ceremony before the first signature")

// ErrCounterpartyNotSettled refuses a signature while this instance holds no
// verified evidence that the OTHER party agreed to the version about to be
// signed (the settlement artifact the peer ships over the DCS-to-DCS channel,
// internal/dcstodcs/settlement.go).
//
// It is deliberately distinct from contractstate.ErrInvalidTransition: that
// one says the signer may not sign, this one says nobody may sign yet. The
// contract is waiting for the counterparty, and the wait ends without anyone
// here doing anything — which is what a viewer must be able to tell a signer
// instead of showing them a dead button.
var ErrCounterpartyNotSettled = errors.New("the counterparty has not settled the version of the contract about to be signed")

// ErrUnknownSignatureField rejects a ceremony/signature for a field the
// contract document does not declare.
var ErrUnknownSignatureField = errors.New("signature field is not declared by the contract document")

// ErrSignatureInvalid rejects a submitted external signature that fails
// validation or whose certificate does not identify the signatory (sole
// control, ADR-12, DCS-FR-SM-16/-18).
var ErrSignatureInvalid = errors.New("submitted signature is not valid or does not identify the signatory")

// ErrFieldAlreadySigned rejects re-signing an already-signed field.
var ErrFieldAlreadySigned = errors.New("signature field is already signed")

// ErrCeremonyNotPrepared rejects a submit for a ceremony that was never
// prepared: submit validates against the bytes pinned at prepare (ADR-20) and
// applies no signature and derives no fresh bytes of its own, so a ceremony
// with nothing pinned has nothing to validate against.
var ErrCeremonyNotPrepared = errors.New("ceremony has no to-be-signed document pinned; call prepare before submit")

// ErrCeremonyConsumed rejects a submit for a ceremony whose signing request
// was already accepted (ADR-20 atomic consumption): the consume guard and the
// finalize writes commit in the SAME transaction, so this can only be lost to
// a genuinely earlier submit, never a race.
var ErrCeremonyConsumed = errors.New("ceremony signing request has already been consumed")

// ErrDocumentMismatch rejects a submitted PDF whose initial revision (the
// bytes before the signatory's incremental PAdES update) is not byte-for-byte
// the document pinned at prepare (ADR-20 TBS byte pinning).
var ErrDocumentMismatch = errors.New("submitted document does not match the document prepared for signing")

// ErrContentMismatch rejects a submitted PDF whose visible page content is no
// longer the page content of the document pinned at prepare. Append-only
// (ErrDocumentMismatch) and content-preserving are different properties: a PDF
// incremental update may redefine ANY object, page content streams included, so
// a submission can be a byte-prefix extension of the prepared document and still
// display different contract text — with a /ByteRange over the whole file, so
// the signature validates over the modified document.
var ErrContentMismatch = errors.New("submitted document's visible content is not the content prepared for signing")

// ErrPayloadMismatch rejects a submitted PDF whose embedded machine-readable
// JSON-LD contract is no longer the payload attached to the document pinned at
// prepare. It is deliberately distinct from ErrContentMismatch: the visible
// pages and the embedded payload are two independent representations of the
// same contract, and an incremental update can supersede the embedded-file
// object alone — leaving every rendered page identical while replacing the
// document that drives policy evaluation, catalogue publication and the peer's
// copy of the contract.
var ErrPayloadMismatch = errors.New("submitted document's machine-readable contract is not the payload prepared for signing")

// ErrNonceMismatch rejects a submitted signature that does not echo the
// ceremony's request nonce, cryptographically bound inside the JAdES
// signature's covered content (ADR-20 nonce binding).
var ErrNonceMismatch = errors.New("submitted signature is not bound to the ceremony's request nonce")

// ErrLevelBelowRequired rejects a submitted signature whose achieved AdES
// level does not meet the contract's declared requirement for the field
// (ADR-20 level-aware acceptance, SM-01).
var ErrLevelBelowRequired = errors.New("submitted signature does not meet the contract's required signature level")

// ErrCertPIDMismatch rejects a submitted signature whose certificate subject
// does not name the ceremony's verified PID (sole control, ADR-20).
var ErrCertPIDMismatch = errors.New("signing certificate does not identify the ceremony's verified signatory")

// ErrCertInconsistent rejects a submitted signature whose certificate does
// not match a certificate the SAME signatory already used elsewhere on this
// contract (ADR-20 cross-ceremony consistency).
var ErrCertInconsistent = errors.New("signing certificate is inconsistent with this signatory's other signatures on this contract")

// ErrJAdESInvalid rejects a submitted JAdES that fails DSS validation.
var ErrJAdESInvalid = errors.New("submitted JAdES signature is invalid")

// ErrRegenerationInFlight reports that the background PDF regenerator still
// holds this contract, so the base document the signature would cover is not
// settled yet. It is a retry-later condition, not a rejection of the caller.
var ErrRegenerationInFlight = errors.New("the contract's PDF is still being regenerated; retry signing shortly")

// regenerationLockWait is how long prepare waits for the background
// regenerator to release a contract. It is a fraction of
// conf.TransactionTimeout on purpose: one regeneration takes seconds, so this
// absorbs a normal queue, while a regenerator wedged on pdf-core or the
// artifact store holds the lock for its own full timeout — which equals the
// whole transaction budget. Waiting that long cannot succeed and would leave
// prepare no budget for its own work, so it would fail anyway, at whatever
// query happened to run first and with a deadline error naming that query
// instead of the contention. Failing here says what actually happened.
const regenerationLockWait = 15 * time.Second

// acquireRegenerationLock takes the per-contract PDF regeneration lock, bounded
// by regenerationLockWait. lock_timeout is scoped to this statement and reset
// afterwards, so the rest of the transaction keeps the caller's own deadline as
// its only bound.
func acquireRegenerationLock(ctx context.Context, tx *sqlx.Tx, did string) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL lock_timeout = %d", regenerationLockWait.Milliseconds())); err != nil {
		return fmt.Errorf("bound the wait for the per-contract PDF regeneration lock for %s: %w", did, err)
	}
	_, lockErr := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", did)
	if _, err := tx.ExecContext(ctx, "SET LOCAL lock_timeout TO DEFAULT"); err != nil && lockErr == nil {
		return fmt.Errorf("restore the lock wait bound after locking %s: %w", did, err)
	}
	if lockErr != nil {
		return regenerationLockError(did, lockErr)
	}
	return nil
}

// assertFieldUnsigned reports ErrFieldAlreadySigned when records already hold a
// SIGNED signature for fieldName.
func assertFieldUnsigned(records []db.SignatureRecord, fieldName string) error {
	for _, rec := range records {
		if rec.Status == "SIGNED" && rec.FieldName != nil && *rec.FieldName == fieldName {
			return fmt.Errorf("%w: %s", ErrFieldAlreadySigned, fieldName)
		}
	}
	return nil
}

// regenerationLockError reports a failed lock acquisition, distinguishing the
// wait that ran out (the regenerator still holds the contract) from any other
// database failure.
func regenerationLockError(did string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == pqLockNotAvailable {
		return fmt.Errorf("%w: %s", ErrRegenerationInFlight, did)
	}
	return fmt.Errorf("acquire per-contract PDF regeneration lock for %s: %w", did, err)
}

// pqLockNotAvailable is the PostgreSQL error code for a lock wait cut short by
// lock_timeout (55P03, lock_not_available).
const pqLockNotAvailable = "55P03"

// ApplyCmd carries the inputs for applying a digital signature.
type ApplyCmd struct {
	DID       string
	SignerDID string
	// FieldName selects which declared signature field this signer covers
	// on a multi-signer contract (DCS-FR-SM-07/-17). Empty = single-signer
	// flow (resolve the signer's most recent verified ceremony).
	FieldName string
	// CeremonyID, when set, resolves this EXACT ceremony instead of "the
	// signer's most recent verified ceremony [for this field]" — the
	// heuristic resolveCeremony falls back to when this is empty. The
	// heuristic is ambiguous once more than one ceremony has been verified
	// for the same signer/field between a Prepare call and a later
	// SubmitSignature call (ADR-20 byte pinning requires the two calls
	// resolve the SAME ceremony; the caller should pass the ceremony_id it
	// already has from starting/polling the ceremony to both calls).
	CeremonyID     string
	CredentialType string
	AppliedBy      string
	HolderDID      string
	UserRoles      userrole.UserRoles
}

// SignatureValidator validates an externally-produced signature and reports the
// signer identity, AdES level, and signing time (dss.Client satisfies it). The
// DCS uses it to accept a signature the signatory produced — never one it made
// itself (ADR-12, DCS-FR-SM-16/-18). That is what sole control requires of US:
// we hold no signing key. It does not by itself prove the signatory controlled
// theirs — that depends on their wallet, and is emphatically untrue of the
// development testWallet, whose keys are shared files.
type SignatureValidator interface {
	ValidatePDF(ctx context.Context, pdf []byte, name string) (*dss.Report, error)
}

// ContentMatcher compares the visible page content of a submitted PDF against a
// reference PDF, resolving the last definition of every object on both sides
// (pdfcore.Client satisfies it). It re-renders nothing: both sides are documents
// the caller already holds.
type ContentMatcher interface {
	MatchContent(ctx context.Context, submitted, reference []byte) (bool, string, error)
}

// assertPreparedContent refuses a submission whose visible pages are no longer
// the prepared document's. The diagnostic the matcher returns names the page
// that diverged and a snippet of both sides, so the refusal says WHAT changed
// rather than that something did.
func assertPreparedContent(ctx context.Context, matcher ContentMatcher, submitted, prepared []byte, fieldName string) error {
	match, mismatch, err := matcher.MatchContent(ctx, submitted, prepared)
	if err != nil {
		return fmt.Errorf("could not content-match the submitted document against the document prepared for %q: %w", fieldName, err)
	}
	if !match {
		return fmt.Errorf("%w: field %q: %s", ErrContentMismatch, fieldName, mismatch)
	}
	return nil
}

// PayloadExtractor returns the machine-readable JSON-LD contract attached to a
// PDF, resolving the LAST definition of the embedded-file object as a reader
// would (pdfcore.Client satisfies it). Like the content matcher it re-renders
// nothing: both documents it is asked about are documents the caller holds.
type PayloadExtractor interface {
	ExtractPayload(ctx context.Context, pdf []byte) ([]byte, error)
}

// assertPreparedPayload refuses a submission whose embedded machine-readable
// contract is no longer the one attached to the pinned prepared document.
//
// The reference is the PINNED PDF's OWN attachment, not the ceremony's pinned
// JAdES payload. Those two are different documents by construction: the pinned
// payload is a JCS-canonicalized envelope built from the contract data as it
// stands at prepare, while the attachment is the raw JSON-LD the renderer
// embedded verbatim — and the two legitimately diverge whenever prepare does
// not re-amend the PDF (every signature after the first, and any contract whose
// base PDF already carries a peer's PAdES signature). Comparing against the
// pinned payload would refuse those valid submissions; comparing the two
// documents' attachments asks exactly the question this gate exists for — did
// the signatory return the machine-readable contract they were handed.
//
// The comparison is over SHA-256 of the extracted attachments rather than the
// bytes themselves so the refusal can name both digests without echoing an
// attacker-supplied document into the error and the audit log.
func assertPreparedPayload(ctx context.Context, extractor PayloadExtractor, submitted, prepared []byte, fieldName string) error {
	submittedPayload, err := extractor.ExtractPayload(ctx, submitted)
	if err != nil {
		return fmt.Errorf("could not read the machine-readable contract embedded in the document submitted for %q: %w", fieldName, err)
	}
	preparedPayload, err := extractor.ExtractPayload(ctx, prepared)
	if err != nil {
		return fmt.Errorf("could not read the machine-readable contract embedded in the document prepared for %q: %w", fieldName, err)
	}
	if len(submittedPayload) == 0 || len(preparedPayload) == 0 {
		return fmt.Errorf("%w: field %q: the documents carry no machine-readable contract to compare", ErrPayloadMismatch, fieldName)
	}
	submittedSum := sha256.Sum256(submittedPayload)
	preparedSum := sha256.Sum256(preparedPayload)
	if submittedSum != preparedSum {
		return fmt.Errorf("%w: field %q: submitted payload sha256 %s, prepared payload sha256 %s",
			ErrPayloadMismatch, fieldName, hex.EncodeToString(submittedSum[:]), hex.EncodeToString(preparedSum[:]))
	}
	return nil
}

// PeerSettlements reads the settlement artifact a counterparty instance ships
// on reaching its own settled state (NEGOTIATION -> SUBMITTED): a JAdES naming
// the contract and the digest of the document that party agreed to, verified
// against the peer's published assertion key before it is stored
// (internal/dcstodcs/settlement.go, internal/service/dcs_to_dcs_settlement.go).
// The same store holds the settlements this instance produced, keyed by its own
// did:web, which is how the gate below knows which version WE agreed to.
type PeerSettlements interface {
	GetSettlement(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) (*dcsdb.Settlement, error)
}

// assertCounterpartiesSettled refuses to sign until this instance holds, for
// every other party the contract declares a signature slot for, that party's
// verified settlement artifact naming the same document this instance itself
// settled.
//
// Deployment already refuses without the peer's shipped SIGNATURE
// (contractworkflowengine/command/deploy.go). That gate sits one step later
// than the one that matters: a signature binds the moment it is made, so
// refusing to deploy afterwards does not undo a commitment to a version the
// counterparty never agreed to. Local state cannot stand in for the peer's —
// ADR-13 keeps intrinsic state local, so this instance can run its own
// submit/review/approve to APPROVED while the peer is still negotiating, which
// is exactly what the live demo instances did.
//
// The version binding is the settlement's document digest, never
// contract_version: that counter is per-instance (the sender bumps it when it
// merges a redline, the receiver on every inbound ship) and says nothing across
// the boundary. The reference digest is the one in THIS instance's own
// settlement row rather than the digest of the contract as it stands now,
// because the first signature seals the offer into an odrl:Agreement — from
// prepare onwards the stored document is no longer the negotiated one, while
// both settlement rows go on naming it. That also keeps the answer stable
// across a second prepare and identical at prepare and at submit.
//
// A contract that declares no remote party's slot has no peer to wait for and
// passes untouched: that is the single-instance multi-signer flow, whose fields
// are named per signatory rather than per party (contractstate.IsRemotePartyField).
func assertCounterpartiesSettled(
	ctx context.Context, tx *sqlx.Tx, settlements PeerSettlements,
	did, localPeer string, resp *db.Responsible, declaredFields []string,
) error {
	var remote []string
	for _, field := range declaredFields {
		if contractstate.IsRemotePartyField(partyDIDs(resp), localPeer, field) {
			remote = append(remote, field)
		}
	}
	if len(remote) == 0 {
		return nil
	}
	if settlements == nil {
		return fmt.Errorf("could not check the counterparty settlement of %s: no settlement store is configured", did)
	}

	own, err := settlements.GetSettlement(ctx, tx, did, localPeer)
	if err != nil {
		return fmt.Errorf("could not read this instance's settlement of %s: %w", did, err)
	}
	// No own settlement means this instance never stated which version it
	// agreed to, and never shipped that statement — so the counterparty's own
	// gate is refusing it in the same way. Refusing here adds no new stuck
	// contract; it just says so at the party that can act on it.
	if own == nil {
		return fmt.Errorf("%w: this instance has not settled %s as %s, so it holds no agreed version to hold %s to",
			ErrCounterpartyNotSettled, did, localPeer, strings.Join(remote, ", "))
	}

	for _, party := range remote {
		settlement, err := settlements.GetSettlement(ctx, tx, did, party)
		if err != nil {
			return fmt.Errorf("could not read the settlement of %s by %s: %w", did, party, err)
		}
		if settlement == nil {
			return fmt.Errorf("%w: no settlement from %s is held", ErrCounterpartyNotSettled, party)
		}
		if settlement.DocumentDigest != own.DocumentDigest {
			return fmt.Errorf("%w: %s settled document %s, this instance settled %s",
				ErrCounterpartyNotSettled, party, settlement.DocumentDigest, own.DocumentDigest)
		}
	}
	return nil
}

// Applier runs the signing command flow: prepare the to-be-signed document,
// and — after the signatory signs it externally (ADR-12) — validate and
// finalize. The DCS holds no contract-signing key.
type Applier struct {
	DB           *sqlx.DB
	CRepo        db.ContractRepo
	CeremonyRepo db.CeremonyRepo
	PDFCore      *pdfcore.Client
	Artifacts    *artifactstore.Store
	VCSigner     provenance.VCSigner
	// VCIssuer issues the C2PA lifecycle-assertion VC stamped into the base
	// PDF before signing (DCS-OR-C2PA-004) — see stampActiveLifecycle below.
	VCIssuer  provenance.VCIssuer
	IssuerDID string
	// StatusEntries hands out the contract's entry in the status list this
	// deployment serves, so the signing summary credentials issued here name the
	// same entry the contract's lifecycle credentials do.
	StatusEntries provenance.StatusListEntries
	// LocalPeer is this instance's own did:web, read from the DID document.
	// It is the identity the DCS-to-DCS channel writes into the settlement
	// artifacts, and the one that tells a counterparty's signature slot from
	// ours — not the separately configured ISSUER_DID, which names the
	// credential issuer and would silently disable both when left unset.
	LocalPeer string
	// Settlements holds the counterparty-settlement evidence the signing gate
	// requires (assertCounterpartiesSettled). Required for a federated
	// contract: without it an instance cannot tell a peer that agreed to this
	// version from one still negotiating.
	Settlements PeerSettlements
	// ArchiveRepo, IPFSStorer, ArchiveNotary, and ArchiveTSA back the
	// archive-entry creation that now happens on reaching SIGNED (DCS-FR-
	// CWE-20), not on APPROVED. ArchiveRepo is the contractworkflowengine
	// repo (same contracts/contract_archive_entries tables as CRepo above,
	// a different package's repo interface) reused purely for its
	// StoreArchiveEntry/ReadDataByDID methods.
	ArchiveRepo   cwedb.ContractRepo
	IPFSStorer    cwecommand.ArchiveSnapshotStorer
	ArchiveNotary cwecommand.ArchiveNotary
	ArchiveTSA    cwecommand.ArchiveTimestampIssuer
	// Validator validates an externally-produced signature (the signatory's
	// wallet/QTSP, or a desktop PAdES signer) before the DCS records it. Required
	// by SubmitSignature; the transitional DCS-signing Handle path does not use it.
	Validator SignatureValidator
}

// Prepare produces the to-be-signed PDF the signatory signs externally — with
// their wallet/QTSP over OID4VP (ADR-12), or by downloading it and signing the
// AcroForm field in a desktop signer such as Adobe Acrobat. It runs the
// pre-signature preparation, embeds the signing-summary evidence inside the byte
// range the external signature will cover (embed-then-sign, ADR-3), and returns
// the unsigned PDF with the signature field placed. The sealed agreement is
// persisted so the content the signatory signs is frozen. The DCS applies no
// signature and holds no signing key.
func (h *Applier) Prepare(ctx context.Context, cmd ApplyCmd) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	prepared, err := h.prepare(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}

	toBeSigned, err := h.PDFCore.EmbedEvidence(ctx, prepared.basePDF, prepared.evidence)
	if err != nil {
		return nil, fmt.Errorf("embed signing evidence: %w", err)
	}

	// Pin the exact to-be-signed bytes and the finalize metadata derived
	// alongside them, at EVERY prepare — wallet ceremony and desktop path
	// alike (ADR-20). SubmitSignature validates against these pinned bytes and
	// never re-derives them, so a submitted document is only ever compared
	// against what THIS prepare committed to, not a freshly re-run pipeline.
	toBeSignedSum := sha256.Sum256(toBeSigned)
	payloadSum := sha256.Sum256(prepared.jadesPayload)
	if err := h.CeremonyRepo.PinPreparedBytes(ctx, tx, db.PinnedBytes{
		CeremonyID:             prepared.ceremony.ID,
		PreparedPDF:            toBeSigned,
		PreparedPDFSHA256:      hex.EncodeToString(toBeSignedSum[:]),
		PinnedPayload:          prepared.jadesPayload,
		PinnedPayloadSHA256:    hex.EncodeToString(payloadSum[:]),
		PinnedContentHash:      prepared.contentHash,
		PinnedRendererVersion:  prepared.rendererVersion,
		PinnedSignedCount:      prepared.signedCount,
		PinnedContractVersion:  prepared.contractVersion,
		RequiredCredentialType: prepared.requiredCredentialType,
	}); err != nil {
		return nil, fmt.Errorf("pin prepared signature bytes: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit prepared signature: %w", err)
	}
	return toBeSigned, nil
}

// SubmitSignatureCmd carries an externally-produced signature over the prepared
// document back to the DCS for validation and recording.
type SubmitSignatureCmd struct {
	ApplyCmd
	// SignedPDF is the PAdES-signed contract the signatory produced.
	SignedPDF []byte
	// JAdESSignature is the signatory's signature over the machine-readable
	// JSON-LD (DCS-FR-SM-02/-11). Empty when only the PDF was signed (e.g. a
	// desktop PAdES signer with no JAdES capability).
	JAdESSignature string
}

// SubmitSignature accepts a signature the signatory produced externally (their
// wallet/QTSP, or a desktop PAdES signer) and finalizes the contract once the
// signature validates and its certificate identifies the signatory (sole
// control, ADR-12, DCS-FR-SM-16/-18). The DCS holds no signing key: it validates
// and records what the signatory returned. This is the same acceptance path for
// the wallet callback and for a downloaded-then-Adobe-signed re-upload — the DCS
// is ignorant of how the signature was produced, only that it is the
// signatory's.
func (h *Applier) SubmitSignature(ctx context.Context, cmd SubmitSignatureCmd) error {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if h.Validator == nil {
		return fmt.Errorf("a signature validator is required to accept an external signature")
	}
	if h.PDFCore == nil {
		return fmt.Errorf("a pdf-core client is required to content-match an external signature against the prepared document")
	}
	if len(cmd.SignedPDF) == 0 {
		return fmt.Errorf("no signed document was submitted")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// SubmitSignature is a pure validate-and-record step (ADR-20): it never
	// re-runs prepare() — no re-sealing the agreement, no re-issuing the
	// summary VC, no re-stamping the C2PA lifecycle. Everything it needs was
	// computed exactly once, at prepare, and pinned on the ceremony; a
	// mismatch against those pinned bytes is the whole of the acceptance
	// check, not a re-derivation to compare against.
	ceremony, err := resolveCeremony(ctx, tx, h.CeremonyRepo, cmd.ApplyCmd)
	if err != nil {
		return err
	}
	if len(ceremony.PreparedPDF) == 0 || ceremony.PinnedContentHash == nil ||
		ceremony.PinnedRendererVersion == nil || ceremony.PinnedSignedCount == nil ||
		ceremony.PinnedContractVersion == nil || ceremony.RequiredCredentialType == nil {
		return ErrCeremonyNotPrepared
	}
	if ceremony.ConsumedAt != nil {
		return ErrCeremonyConsumed
	}

	// Safety net against staleness between prepare and submit: the contract
	// must still be in a state signing is valid from, the counterparty must
	// still be settled on the version this instance agreed, and this field must
	// still be unsigned. Re-checking is a cheap read, not a re-derivation of
	// prepare's business logic (sealing, evidence, SHACL/policy gates).
	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}
	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventSign); err != nil {
		return err
	}
	// A settlement the peer replaced between prepare and submit — because it
	// reopened the negotiation and settled a different document — leaves the
	// pinned bytes covering a version nobody agreed to any more. The digest
	// comparison catches that; it is not a repeat of prepare's answer.
	var declaredFields []string
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		declaredFields = validation.RequiredSignatureFields(*data.ContractData)
	}
	if err := assertCounterpartiesSettled(ctx, tx, h.Settlements, cmd.DID, h.LocalPeer, data.Responsible, declaredFields); err != nil {
		return err
	}
	existingRecords, err := h.CRepo.LoadSignatures(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not load existing signatures: %w", err)
	}
	if err := assertFieldUnsigned(existingRecords, ceremony.FieldName); err != nil {
		return err
	}

	// TBS byte pinning (ADR-20): a submitted PDF may only ADD a PAdES
	// incremental update to the document prepare committed to — it may never
	// redefine any byte of it. PAdES signing is itself an incremental update,
	// so "the same document, plus our own signature" is exactly a byte-prefix
	// relationship; anything else (tampered visible pages, a substituted
	// document with only the attachment reused) fails this check outright,
	// independently of whether the signature itself validates.
	if !bytes.HasPrefix(cmd.SignedPDF, ceremony.PreparedPDF) {
		return fmt.Errorf("%w: the submitted PDF's initial revision is not the document prepared for signing", ErrDocumentMismatch)
	}

	// Content pinning, the second half of the same guarantee: the prefix check
	// above proves the submission only APPENDED, which is not the same as
	// leaving the document unmodified — an appended revision may supersede a
	// page content stream and change what the contract says while the signature
	// covers the whole file and validates. Compare the visible pages of the
	// submission against the visible pages of the PINNED prepared bytes, which
	// resolves the LAST definition of every object, so a superseding revision
	// diverges. Nothing is re-rendered: the reference is the exact document
	// prepare committed to, so this carries none of the render-determinism
	// fragility a fresh compile would (ADR-13).
	if err := assertPreparedContent(ctx, h.PDFCore, cmd.SignedPDF, ceremony.PreparedPDF, ceremony.FieldName); err != nil {
		return err
	}

	// Payload pinning, the machine-readable half of the same guarantee. The two
	// checks above say nothing about the embedded JSON-LD: a revision that
	// supersedes ONLY the embedded-file object leaves every rendered page
	// byte-identical, so the content match passes, while the document that
	// actually drives policy evaluation, catalogue publication and the peer's
	// copy of the contract has been swapped under a signature covering the whole
	// file. Compare the attachment a reader resolves in the submission against
	// the attachment of the pinned prepared bytes — again re-rendering nothing.
	if err := assertPreparedPayload(ctx, h.PDFCore, cmd.SignedPDF, ceremony.PreparedPDF, ceremony.FieldName); err != nil {
		return err
	}

	report, err := h.Validator.ValidatePDF(ctx, cmd.SignedPDF, ceremony.FieldName)
	if err != nil {
		return fmt.Errorf("validate submitted signature: %w", err)
	}
	if err := report.AssertValidAES(); err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	requiredLevel := strings.ToUpper(strings.TrimSpace(*ceremony.RequiredCredentialType))
	achievedQES := report.AssertValidQES() == nil
	if requiredLevel == "QES" && !achievedQES {
		return fmt.Errorf("%w: contract requires QES for %q, submitted signature is %s/%s (qualification %q)",
			ErrLevelBelowRequired, ceremony.FieldName, report.Indication, report.SubIndication, report.Qualification)
	}
	achievedLevel := "AES"
	if achievedQES {
		achievedLevel = "QES"
	}

	// Sole control (eIDAS Art. 26c): the certificate must identify the
	// ceremony's verified PID by name — mandatory for QES (Annex I requires
	// the qualified cert to carry the signatory's verified name), policy-
	// configurable for AES (ADR-20; defaults to enforced).
	//
	// The certificate identity is read directly from the submitted PDF's own
	// CMS SignerInfo (signerCertificateFromIncrementalUpdate), not from DSS's
	// validation report: DSS's simpleReport never carries structured
	// GIVENNAME/SURNAME at all (only a CommonName-derived SignedBy), and its
	// diagnosticData per-certificate entries came back empty in CI for our
	// dev CA - DSS appears to only populate those for certificates it
	// recognizes as qualified. Reading the bytes this submission itself
	// added (ADR-20 byte pinning already proves cmd.SignedPDF is exactly
	// ceremony.PreparedPDF plus one incremental update) has no such
	// dependency on what DSS chooses to report, and cannot be ambiguous
	// about which signature it names the way DSS's report was before
	// resolveSigningCertificate/latestSignatureEntry scoped it.
	pidGiven, pidFamily := pidGivenFamilyName(ceremony.PidClaims)
	nameMatchRequired := requiredLevel == "QES" || conf.AESCertNameMatchRequired()
	// Read directly, unconditionally: this is also the source of the
	// certificate SERIAL NUMBER recorded as sole-control evidence below
	// (DCS-FR-SM-26), which callers need regardless of whether the name-match
	// gate itself is administratively enforced for this signature.
	signerCert, err := signerCertificateFromIncrementalUpdate(cmd.SignedPDF, ceremony.PreparedPDF)
	if err != nil {
		return fmt.Errorf("%w: could not read the submitted signature's own certificate: %v", ErrCertPIDMismatch, err)
	}
	var certGiven, certSurname string
	if nameMatchRequired {
		certGiven, certSurname = certGivenSurname(signerCert)
	}
	// The sole-control gate's outcome is a security decision on every
	// submission, and the callback response alone carries no detail about
	// what was actually compared — logging it is cheap and turns "why was
	// this signature accepted/rejected" into real, immediate evidence
	// instead of needing to reproduce it.
	log.Printf(
		"sole-control name-match: ceremony=%s field=%s requiredLevel=%s nameMatchRequired=%v pid=%q/%q cert=%q/%q rawSignedBy=%q",
		ceremony.ID, ceremony.FieldName, requiredLevel, nameMatchRequired, pidGiven, pidFamily, certGiven, certSurname, report.SignedBy,
	)
	if nameMatchRequired && !namesMatch(pidGiven, pidFamily, certGiven, certSurname) {
		return fmt.Errorf("%w: PID %q %q vs. certificate %q %q", ErrCertPIDMismatch, pidGiven, pidFamily, certGiven, certSurname)
	}
	// Cross-ceremony consistency: this signatory must use the SAME
	// certificate across every signature they have already placed on this
	// contract (a mid-contract certificate swap for one signer is exactly the
	// signal a compromised or shared key would produce).
	for _, rec := range existingRecords {
		if rec.SignerDID != cmd.SignerDID || rec.CeremonyID == nil {
			continue
		}
		priorCeremony, err := h.CeremonyRepo.GetCeremonyByID(ctx, tx, *rec.CeremonyID)
		if err != nil {
			return fmt.Errorf("could not resolve prior ceremony %s: %w", *rec.CeremonyID, err)
		}
		if priorCeremony != nil && priorCeremony.SignerCertSubject != nil &&
			normalizeName(*priorCeremony.SignerCertSubject) != normalizeName(report.SignedBy) {
			return fmt.Errorf("%w: prior certificate %q vs. this certificate %q", ErrCertInconsistent, *priorCeremony.SignerCertSubject, report.SignedBy)
		}
	}

	// Nonce binding (ADR-20): a wallet-ceremony callback (a published request
	// with a fresh nonce) requires the JAdES signature over the machine-
	// readable payload, with the ceremony's request nonce cryptographically
	// bound inside its protected header — the covered content DSS already
	// validated the signature over. A holder of just the ceremony URL cannot
	// forge this without the signatory's own key. The desktop path (never
	// published, no request nonce) has no nonce to bind; its JAdES, if any,
	// is still validated below.
	nonceRequired := ceremony.RequestNonce != nil
	jades := strings.TrimSpace(cmd.JAdESSignature)
	if nonceRequired && jades == "" {
		return fmt.Errorf("%w: a wallet-ceremony signature requires a JAdES over the machine-readable payload to bind the request nonce", ErrNonceMismatch)
	}
	if jades != "" {
		jadesReport, err := h.Validator.ValidatePDF(ctx, []byte(jades), ceremony.FieldName+"-payload.json")
		if err != nil {
			return fmt.Errorf("validate submitted JAdES: %w", err)
		}
		if err := jadesReport.AssertValidAES(); err != nil {
			return fmt.Errorf("%w: %v", ErrJAdESInvalid, err)
		}
		payload, err := jwsPayloadBytes(jades)
		if err != nil {
			return fmt.Errorf("%w: decode JAdES payload: %v", ErrJAdESInvalid, err)
		}
		if len(ceremony.PinnedPayload) > 0 && !bytes.Equal(payload, ceremony.PinnedPayload) {
			return fmt.Errorf("%w: JAdES payload is not the machine-readable document prepared for signing", ErrDocumentMismatch)
		}
		if nonceRequired {
			nonceClaim, err := jwsProtectedHeaderClaim(jades, "nonce")
			if err != nil {
				return fmt.Errorf("%w: %v", ErrNonceMismatch, err)
			}
			if nonceClaim == "" || nonceClaim != *ceremony.RequestNonce {
				return fmt.Errorf("%w: JAdES carries no matching request nonce", ErrNonceMismatch)
			}
		}
	}

	// Everything above only read, to decide whether the submission is
	// acceptable; everything below writes. The per-contract regeneration lock —
	// the key prepare() and the background regenerator take — covers the write
	// half, and is taken here, at the boundary.
	//
	// It covers the writes because the regenerator's decision to leave a
	// contract alone reads the signature row and the PDF pointer written below,
	// and it holds this lock from that read to its own commit: serialising the
	// two writers settles the order either way round. The regenerator commits
	// first and the signed CID lands after its render, or this transaction
	// commits first and the regenerator then reads the signature and declines.
	//
	// It stops at the boundary because the DSS round trips above are seconds of
	// external validation the regenerator has no stake in. Spanning them, this
	// lock blocked the regeneration that this contract's own prepare() had just
	// triggered — on the event handler every other contract's regeneration
	// queues behind — for the whole validation window, which cost the BDD suite
	// three times its runtime in stalled exports, stalled peer ships and
	// ErrRegenerationInFlight. This is still the first lock the transaction
	// takes, so the order that keeps it deadlock-free is unchanged.
	if err := acquireRegenerationLock(ctx, tx, cmd.DID); err != nil {
		return err
	}

	// The already-signed guard, re-read now that the writers are serialised. The
	// read above it runs before the DSS round trips, so two submits for the same
	// field can both pass it: a field can have two verified unconsumed
	// ceremonies at once — the wallet callback pins the ceremony from its
	// callback URL while FindVerifiedCeremonyByField returns the newest — so
	// each submit consumes its OWN ceremony and MarkCeremonyConsumed's guard
	// stops neither. Both would then write a SIGNED row for one field, the
	// second SetSignedPDF would drop the first signature from the stored bytes,
	// and both would archive. Under the lock this read sees the winner's
	// committed row, so the loser stops here with ErrFieldAlreadySigned. The
	// earlier read stays: it costs one query and saves the whole DSS window on
	// the ordinary already-signed submission.
	serialisedRecords, err := h.CRepo.LoadSignatures(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not re-load existing signatures: %w", err)
	}
	if err := assertFieldUnsigned(serialisedRecords, ceremony.FieldName); err != nil {
		return err
	}

	// Atomic consumption (ADR-20): the guarded UPDATE ... WHERE consumed_at IS
	// NULL and the finalize writes below commit or roll back TOGETHER, in this
	// one transaction. Two concurrent submits for the same ceremony can never
	// both finalize — the second one's guard sees consumed_at already set (by
	// the first submit's still-uncommitted-but-serialized write, or its
	// committed one) and this whole transaction rolls back.
	if err := h.CeremonyRepo.MarkCeremonyConsumed(ctx, tx, ceremony.ID); err != nil {
		return fmt.Errorf("%w: %v", ErrCeremonyConsumed, err)
	}
	if err := h.CeremonyRepo.RecordSignerCertificate(ctx, tx, ceremony.ID, report.SignedBy, hex.EncodeToString(signerCert.SerialNumber.Bytes())); err != nil {
		return err
	}

	signedAt := time.Now().UTC()
	if t, perr := time.Parse(time.RFC3339, report.SigningTime); perr == nil {
		signedAt = t.UTC()
	}
	applyCmd := cmd.ApplyCmd
	applyCmd.CredentialType = achievedLevel

	if err := h.finalize(ctx, tx, applyCmd, finalizeInput{
		ceremony:        ceremony,
		signedPDF:       cmd.SignedPDF,
		jadesSignature:  cmd.JAdESSignature,
		contentHash:     *ceremony.PinnedContentHash,
		rendererVersion: *ceremony.PinnedRendererVersion,
		signedCount:     *ceremony.PinnedSignedCount,
		vpToken:         derefStr(ceremony.VpToken),
		kbSDHash:        derefStr(ceremony.KbSdHash),
		signedAt:        signedAt,
		contractVersion: *ceremony.PinnedContractVersion,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// signingEvidenceAttachment packages ONE ceremony's authorization for embedding:
// the summary this instance issued for it and the Power of Attorney the
// signatory presented at it. A ceremony that presented none carries no field, so
// the receiver reads absence as absence rather than as an empty credential.
func signingEvidenceAttachment(summary json.RawMessage, poaVPToken *string) ([]byte, error) {
	presentation := ""
	if poaVPToken != nil {
		presentation = *poaVPToken
	}
	return json.Marshal(provenance.SigningEvidenceAttachment{Summary: summary, PoAPresentation: presentation})
}

// preparedSignature is the to-be-signed material the prepare phase yields: the
// base PDF (AcroForm signature field placed, lifecycle-stamped, NOT yet
// evidence-embedded or signed), the signing-summary evidence to embed, and the
// canonical JAdES payload — plus the ceremony and hashes finalize binds. In the
// wallet-driven ceremony (ADR-12) the base PDF is evidence-embedded and handed
// to the signatory's wallet/QTSP to sign; the DCS applies no signature here.
type preparedSignature struct {
	ceremony    *db.SignatureCeremony
	basePDF     []byte
	basePDFHash string
	// evidence is this ceremony's own signing evidence attachment: its summary
	// credential and the Power of Attorney presented at it.
	evidence        []byte
	jadesPayload    []byte
	contentHash     string
	signedCount     int
	rendererVersion string
	vpToken         string
	kbSDHash        string
	signedAt        time.Time
	contractVersion int
	// requiredCredentialType is the contract's declared level requirement for
	// the ceremony's field (SM-01, ADR-20), pinned so submit gates on it.
	requiredCredentialType string
}

// credentialTypeAtLeast reports whether offered meets or exceeds required on
// the SES < AES < QES scale. Unrecognized values rank as SES (the strictest
// reading: an unrecognized offered level satisfies nothing but SES).
func credentialTypeAtLeast(offered, required string) bool {
	return credentialTypeRank(offered) >= credentialTypeRank(required)
}

func credentialTypeRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "QES":
		return 2
	case "AES":
		return 1
	default:
		return 0
	}
}

// prepare runs every step up to (but not including) the signature: it enforces
// the ceremony precondition and multi-signer gating, seals the offer into the
// odrl:Agreement on the first signature, runs the policy/closedness/conformance
// and SHACL gates, loads and lifecycle-stamps the base PDF, and issues the
// signing-summary credential(s). It mutates within tx (the sealed agreement is
// persisted) but applies no signature and stores no artefact — the caller
// either signs (transitional Handle) or embeds the evidence and hands the PDF
// to the signatory's wallet (the ceremony download).
func (h *Applier) prepare(ctx context.Context, tx *sqlx.Tx, cmd ApplyCmd) (*preparedSignature, error) {
	// Serialize against the background PDF regenerator on the same per-contract
	// key it uses (pdfgeneration/event). Without this, a genesis/lifecycle
	// regeneration already in flight — holding this lock across its slow
	// pdf-core render — commits its UpdatePDFState *after* SetSignedPDF and
	// overwrites the signed CID with an unsigned re-render, stripping the PAdES
	// signature. Waiting here lets the regenerator finish first; the signed
	// state we then write is frozen, so its later events short-circuit.
	if err := acquireRegenerationLock(ctx, tx, cmd.DID); err != nil {
		return nil, err
	}

	data, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", cmd.DID, err)
	}
	if data.ContractData == nil {
		return nil, fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}

	// Ceremony precondition (DCS-FR-SM-16): a completed (verified) PID
	// presentation for this signer and contract must exist. Evaluated before
	// the state-machine transition so a missing ceremony is reported as its own
	// typed error rather than a state error.
	ceremony, err := resolveCeremony(ctx, tx, h.CeremonyRepo, cmd)
	if err != nil {
		return nil, err
	}

	// The contract's OWN declared signature-level requirement for this field
	// (SM-01 per-contract level enforcement, ADR-20) — pinned below and
	// enforced at submit regardless of what credential_type the caller here
	// asked for. Failing fast here too is a UX courtesy, not the gate: the
	// gate is applying the requirement at submit against the level the
	// signature ACTUALLY achieved (see SubmitSignature/dss.AssertMeetsLevel).
	requiredCredentialType := validation.RequiredCredentialType(*data.ContractData, ceremony.FieldName)
	if !credentialTypeAtLeast(cmd.CredentialType, requiredCredentialType) {
		return nil, fmt.Errorf("%w: contract requires %s for %q, ceremony requested %s", ErrLevelBelowRequired, requiredCredentialType, ceremony.FieldName, cmd.CredentialType)
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(data.State), contractstate.EventSign); err != nil {
		return nil, err
	}

	// Multi-signer workflow (DCS-FR-SM-07/-17): contracts that declare
	// signature fields require one ceremony+signature per field, applied
	// SEQUENTIALLY (parallel signing is incompatible with PDF/A-3
	// incremental updates — see the change request), with every ceremony
	// completed BEFORE the first signature so all signers' evidence is
	// embedded ahead of the signature that freezes the document.
	requiredFields := validation.RequiredSignatureFields(*data.ContractData)

	// Signing is a mutual milestone: it claims both parties settled this
	// version. Checked before anything below mutates or persists, so a contract
	// the counterparty has not settled leaves prepare with no trace.
	if err := assertCounterpartiesSettled(ctx, tx, h.Settlements, cmd.DID, h.LocalPeer, data.Responsible, requiredFields); err != nil {
		return nil, err
	}

	existingRecords, err := h.CRepo.LoadSignatures(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not load existing signatures: %w", err)
	}
	signedCount := 0
	for _, rec := range existingRecords {
		if rec.Status != "SIGNED" {
			continue
		}
		signedCount++
		if rec.FieldName != nil && *rec.FieldName == ceremony.FieldName {
			return nil, fmt.Errorf("%w: %s", ErrFieldAlreadySigned, ceremony.FieldName)
		}
	}
	if len(requiredFields) > 0 {
		declared := false
		for _, f := range requiredFields {
			if f == ceremony.FieldName {
				declared = true
				break
			}
		}
		if !declared {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSignatureField, ceremony.FieldName)
		}
		if signedCount == 0 {
			var missing []string
			for _, f := range requiredFields {
				// A peer DCS's slot is signed in the peer's own deployment and
				// its signature arrives over the PDF exchange (ADR-13), so its
				// ceremony evidence never exists in this database. Demanding it
				// here made federated signing impossible: neither side could
				// ever place the first signature. Locally held fields — the
				// single-instance multi-signer flow, which names fields per
				// signatory rather than per party DCS — are unaffected.
				//
				// This gate asks a different question from the deployment gate
				// (contractworkflowengine/command/deploy.go), which shares the
				// same notion of whose slot a field is but demands the peer's
				// shipped signature for it. Here there is nothing to demand:
				// a peer ships that signature only once its copy is SIGNED,
				// which is after this point, not before it.
				if contractstate.IsRemotePartyField(partyDIDs(data.Responsible), h.LocalPeer, f) {
					continue
				}
				c, err := h.CeremonyRepo.FindVerifiedCeremonyByField(ctx, tx, cmd.DID, f)
				if err != nil {
					return nil, fmt.Errorf("could not resolve ceremony for field %q: %w", f, err)
				}
				if c == nil {
					missing = append(missing, f)
				}
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("%w: missing ceremonies for %v", ErrCeremoniesIncomplete, missing)
			}
		}
	}

	// The stored artifact is read ONCE, here, because two decisions below hang on
	// the same question — may this signature still change the machine-readable
	// contract, and does the lifecycle stamp re-embed it — and they must not be
	// able to answer it differently. It is also the base PDF the signature covers.
	storedPDF, err := h.CRepo.FetchContractPDFBytes(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("fetch contract PDF: %w", err)
	}
	// signedCount does not express this across a federation: the counterparty's
	// database holds no row for the originator's signature, so its own
	// countersignature reads as the first one — over a base PDF that already
	// carries the originator's PAdES. The artifact answers for itself.
	artifactFrozen := signedCount > 0 || provenance.CarriesPAdESSignature(storedPDF)

	poaOrganization := ""
	if ceremony.PoAOrganization != nil {
		poaOrganization = *ceremony.PoAOrganization
	}
	document, mutated, err := contractDocumentForSignature(
		*data.ContractData, data.Responsible, cmd.SignerDID, poaOrganization, ceremony.FieldName, artifactFrozen)
	if err != nil {
		return nil, err
	}
	if mutated {
		if err := h.CRepo.UpdateContractData(ctx, tx, cmd.DID, document); err != nil {
			return nil, fmt.Errorf("persist sealed agreement and signatory attribution: %w", err)
		}
		data.ContractData = &document
	}

	if err := validation.ValidateContractPolicySatisfaction(
		*data.ContractData,
		validation.ContractContentAuditMetadata{
			ContractDID:     cmd.DID,
			ContractVersion: fmt.Sprint(data.ContractVersion),
			AuditedBy:       cmd.AppliedBy,
			HolderDID:       cmd.HolderDID,
		},
	); err != nil {
		return nil, err
	}

	// Signatures are the point of no return: a contract must be closed — no
	// unresolved placeholders — before it is sealed into an odrl:Agreement and
	// signed. A template's open policy is only ever a contract once every
	// placeholder is materialized.
	if err := validation.ValidateContractClosed(*data.ContractData); err != nil {
		return nil, fmt.Errorf("signature application blocked: %w", err)
	}

	// A non-conformant contract must never be signed — submission already gates
	// this, but signatures are the point of no return, so the invariant is
	// re-checked here. This is the SHACL hub-conformance gate ADR-24 explicitly
	// retains; it is not DCS-FR-PACM-03, which ADR-24 descopes (that is the
	// autonomous *legal*-conformity assessment, which this check does not make).
	if err := validation.RequireHubConformance(ctx, *data.ContractData); err != nil {
		return nil, fmt.Errorf("signature application blocked: %w", err)
	}

	// SHACL evidence (Phase 4, ADR-9): the hub schema version this contract
	// validates against and a stable hash of the resulting findings, bound
	// into the signing-summary credential below — an external verifier
	// resolves sh:shapesGraph to fetch those exact pinned shapes, re-runs
	// validation, and compares hashes to detect drift.
	schemaVersion, validationReportHash, err := validation.SHACLEvidence(ctx, *data.ContractData)
	if err != nil {
		return nil, fmt.Errorf("SHACL evidence for signing-summary credential: %w", err)
	}

	basePDF, err := h.loadBasePDF(ctx, storedPDF, *data.ContractData)
	if err != nil {
		return nil, err
	}

	// Stamp the "active" C2PA lifecycle assertion into the base PDF BEFORE
	// signing it (update-then-sign), not after. Re-stamping a lifecycle
	// assertion after signing rewrites a referenced embedded-file object (the
	// C2PA manifest attachment), which standards-compliant PAdES validators
	// (Adobe Reader, pyHanko's diff-analysis) flag as an unexplained
	// modification even though the CMS signature itself stays cryptographically
	// valid. Stamping here means the signature commits to the PDF's FINAL
	// lifecycle-bearing content, so exportcontract.go/verifycontract.go never
	// need to touch it again for the SIGNED/ACTIVE C2PA state.
	//
	// A PDF that already carries a PAdES signature is therefore never stamped
	// again — it was stamped before the FIRST signature. This is also the pass
	// that re-embeds the machine-readable contract into the PDF, which is why
	// the document above is only allowed to change when this runs.
	//
	// This is NOT an "a signed PDF is never appended to again" rule, and must
	// not be read as one: finalize() below appends a provenance-only manifest
	// after signing (ADR-26), and further signatures are appended as incremental
	// updates. DCS-OR-C2PA-002 requires exactly that ("It MUST use PDF
	// incremental updates so existing legal signatures remain valid",
	// verification method "Sign→append→verify test") and DCS-OR-C2PA-010
	// requires appends not to break existing signatures. What is scoped here is
	// only the lifecycle stamp. (Earlier revisions cited DCS-OR-C2PA-004 and
	// DCS-FR-SM-16 for an absolute no-mutation rule; neither requirement
	// addresses post-signature mutation — -004 is VC binding, SM-16 is applying
	// the signature.)
	rendererVersion := ""
	if !artifactFrozen {
		stampedPDF, rv, err := stampLifecycleForSigning(ctx, cmd.DID, *data.ContractData, basePDF, h.PDFCore, h.VCIssuer, h.IssuerDID)
		if err != nil {
			return nil, fmt.Errorf("stamp active lifecycle assertion before signing: %w", err)
		}
		basePDF = stampedPDF
		rendererVersion = rv
	}

	contentSum := sha256.Sum256(*data.ContractData)
	contentHash := hex.EncodeToString(contentSum[:])
	basePDFSum := sha256.Sum256(basePDF)
	basePDFHash := hex.EncodeToString(basePDFSum[:])

	// Issue the signing-summary credential carrying the verbatim PID
	// presentation, to be embedded before signing (embed-first-sign-second).
	vpToken := ""
	if ceremony.VpToken != nil {
		vpToken = *ceremony.VpToken
	}
	kbSDHash := ""
	if ceremony.KbSdHash != nil {
		kbSDHash = *ceremony.KbSdHash
	}
	signedAt := time.Now().UTC()

	// The contract's entry in the status list this deployment serves. Read back
	// rather than derived, so every summary credential this ceremony issues names
	// the entry the contract's lifecycle credentials already advertise and one
	// revocation covers all of them (ADR-34).
	if h.StatusEntries == nil {
		return nil, fmt.Errorf("no status list entries source: the signing summary for %s would carry no revocation entry", cmd.DID)
	}
	statusEntry, err := h.StatusEntries.EntryFor(ctx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("status list entry for %s: %w", cmd.DID, err)
	}

	// Every signature carries its OWN authorization inside the artifact it signs
	// (ADR-13, ADR-35): one attachment per signing event, holding the summary
	// this ceremony produced and the Power of Attorney the signatory presented
	// at it, embedded before this signature is applied so this signature covers
	// it. On a frozen inbound artifact the embed is an append-only incremental
	// update, which leaves the signatures already there valid
	// (DCS-OR-C2PA-002) — the countersignature and the ADR-26 reanchor append
	// to the same document.
	summary, _, err := provenance.IssueSigningSummaryVC(ctx, h.VCSigner, h.IssuerDID, statusEntry, provenance.SigningSummary{
		ContractID:           cmd.DID,
		SignerDID:            cmd.SignerDID,
		CeremonyID:           ceremony.ID,
		FieldName:            ceremony.FieldName,
		ContentHash:          contentHash,
		PDFHash:              basePDFHash,
		CredentialType:       cmd.CredentialType,
		KBSDHash:             kbSDHash,
		SignedAt:             signedAt,
		SchemaVersion:        schemaVersion,
		ValidationReportHash: validationReportHash,
	})
	if err != nil {
		return nil, fmt.Errorf("issue signing-summary VC for field %q: %w", ceremony.FieldName, err)
	}
	// Retained as well as embedded: the compliance viewer reads the attestation
	// per ceremony, and on a frozen document this summary is also where the
	// signatory and the authority behind this signature are recorded, since
	// contractDocumentForSignature left dcs:parties alone.
	if err := h.CeremonyRepo.RecordSummaryVC(ctx, tx, ceremony.ID, summary); err != nil {
		return nil, err
	}
	evidence, err := signingEvidenceAttachment(summary, ceremony.PoAVpToken)
	if err != nil {
		return nil, fmt.Errorf("encode signing evidence for field %q: %w", ceremony.FieldName, err)
	}

	// The JAdES payload over the machine-readable JSON-LD, the counterpart to
	// the visible PAdES on the PDF: one signature event covers both
	// representations (DCS-FR-SM-02, DCS-FR-SM-11), so an external verifier can
	// validate the contract's terms from the canonical JSON-LD without the PDF.
	jadesPayload, err := jades.BuildContractPayload(cmd.DID, data.ContractVersion, *data.ContractData)
	if err != nil {
		return nil, fmt.Errorf("build JAdES payload: %w", err)
	}

	return &preparedSignature{
		ceremony:               ceremony,
		basePDF:                basePDF,
		basePDFHash:            basePDFHash,
		evidence:               evidence,
		jadesPayload:           jadesPayload,
		contentHash:            contentHash,
		signedCount:            signedCount,
		rendererVersion:        rendererVersion,
		vpToken:                vpToken,
		kbSDHash:               kbSDHash,
		signedAt:               signedAt,
		contractVersion:        data.ContractVersion,
		requiredCredentialType: requiredCredentialType,
	}, nil
}

// resolveCeremony finds the verified ceremony a signature command applies to.
// On a multi-signer contract several fields may share one signer identity
// (e.g. one person signing two roles), so resolving by signer alone is
// ambiguous — FieldName disambiguates when provided; otherwise it falls back
// to the signer's most recent verified ceremony (single-signer flow). Shared
// by prepare() and SubmitSignature so both resolve identically.
func resolveCeremony(ctx context.Context, tx *sqlx.Tx, repo db.CeremonyRepo, cmd ApplyCmd) (*db.SignatureCeremony, error) {
	if cmd.CeremonyID != "" {
		ceremony, err := repo.GetCeremonyByID(ctx, tx, cmd.CeremonyID)
		if err != nil {
			return nil, fmt.Errorf("could not resolve signing ceremony %s: %w", cmd.CeremonyID, err)
		}
		if ceremony == nil || ceremony.Status != db.CeremonyVerified || ceremony.ContractDID != cmd.DID ||
			ceremony.SignerDID == nil || *ceremony.SignerDID != cmd.SignerDID {
			return nil, ErrCeremonyRequired
		}
		return ceremony, nil
	}

	var ceremony *db.SignatureCeremony
	var err error
	if cmd.FieldName != "" {
		ceremony, err = repo.FindVerifiedCeremonyByField(ctx, tx, cmd.DID, cmd.FieldName)
	} else {
		ceremony, err = repo.FindVerifiedCeremony(ctx, tx, cmd.DID, cmd.SignerDID)
	}
	if err != nil {
		return nil, fmt.Errorf("could not resolve signing ceremony: %w", err)
	}
	if ceremony == nil {
		return nil, ErrCeremonyRequired
	}
	return ceremony, nil
}

// finalizeInput carries the post-signature state the Finalizer persists: the
// wallet-signed PDF, the JAdES over the machine-readable JSON-LD, and the
// hashes/ceremony metadata bound into the signature record and archive entry.
type finalizeInput struct {
	ceremony        *db.SignatureCeremony
	signedPDF       []byte
	jadesSignature  string
	contentHash     string
	rendererVersion string
	signedCount     int
	vpToken         string
	kbSDHash        string
	signedAt        time.Time
	contractVersion int
}

// finalize persists a completed signature: it stores the signed PDF in IPFS,
// points the contract at it, records the signature (PAdES hash + JAdES),
// transitions to SIGNED, and — on the first signature — archives the contract.
// In the wallet-driven ceremony the signedPDF and jadesSignature originate from
// the signatory's wallet/QTSP (the DCS holds no signing key); this is the
// receive-and-record half the ceremony callback invokes after validating the
// returned signature.
func (h *Applier) finalize(ctx context.Context, tx *sqlx.Tx, cmd ApplyCmd, in finalizeInput) error {
	// Re-anchor provenance over the signature (ADR-26). The lifecycle manifest
	// was written before signing so the signature commits to it, which leaves
	// that manifest's whole-file C2PA binding covering less than the signed
	// file. Appending a provenance-only manifest here restores the binding
	// without touching the signature's byte range, so the signature keeps
	// verifying in external tools; PDF readers report the document as modified
	// after signing, which is what happened.
	if h.PDFCore != nil {
		reanchored, err := h.PDFCore.Reanchor(ctx, in.signedPDF, provenance.RemoteManifestURL(cmd.DID))
		if err != nil {
			return fmt.Errorf("re-anchor provenance over the signature for %s: %w", cmd.DID, err)
		}
		in.signedPDF = reanchored
	}

	signedPDFSum := sha256.Sum256(in.signedPDF)
	signedPDFHash := hex.EncodeToString(signedPDFSum[:])

	cid, err := h.Artifacts.Put(ctx, artifactstore.ContractScope(cmd.DID), in.signedPDF)
	if err != nil {
		return fmt.Errorf("store signed PDF in IPFS: %w", err)
	}

	// Confirm the artefact resolves through the read path before persisting its
	// CID. The tenant store is eventually consistent, so a CID the store has
	// just returned is not always immediately retrievable; persisting it early
	// would let a later export/verify fetch the contract's PDF and fail
	// (DCS-FR-SM-16). The underlying fetch retries the transient window.
	readback, err := h.Artifacts.Get(ctx, artifactstore.ContractScope(cmd.DID), cid)
	if err != nil || len(readback) == 0 {
		return fmt.Errorf("signed PDF CID %s not resolvable after store: %w", cid, err)
	}

	// contentHash (computed from *data.ContractData) is the same payload hash
	// exportcontract.go/verifycontract.go compare against, so recording it here
	// means the first export/verify after signing sees a matching hash and
	// serves the frozen signed PDF as-is instead of appending a post-signature
	// revision.
	if err := h.CRepo.SetSignedPDF(ctx, tx, cmd.DID, cid, in.rendererVersion, "active", in.contentHash); err != nil {
		return err
	}

	ceremonyID := in.ceremony.ID
	fieldName := in.ceremony.FieldName
	signature := db.ContractSignature{
		ContractDID:    cmd.DID,
		Status:         "SIGNED",
		SignatureBytes: signedPDFSum[:],
		SignerDID:      cmd.SignerDID,
		CredentialType: cmd.CredentialType,
		IpfsCID:        &cid,
		CeremonyID:     &ceremonyID,
		PDFHash:        &signedPDFHash,
		ContentHash:    &in.contentHash,
		FieldName:      &fieldName,
		JAdESSignature: &in.jadesSignature,
	}
	if err := h.CRepo.CreateSignature(ctx, tx, signature); err != nil {
		return fmt.Errorf("could not create signature: %w", err)
	}

	if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Signed.String()); err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	// The archive entry is created when the contract REACHES SIGNED (first
	// signature); later multi-signer signatures update the stored artefact
	// pointer above but never insert a second entry for the same version.
	if in.signedCount == 0 {
		credentialHashes := map[string]string{}
		if in.vpToken != "" {
			sum := sha256.Sum256([]byte(in.vpToken))
			credentialHashes["presentation"] = "sha256:" + hex.EncodeToString(sum[:])
		}
		if in.kbSDHash != "" {
			credentialHashes["key_binding"] = "sha256:" + strings.TrimPrefix(in.kbSDHash, "sha256:")
		}
		if err := h.archiveSignedContract(ctx, tx, cmd.DID, cmd.AppliedBy, cwecommand.ArchiveSigningEvidence{Signer: cmd.SignerDID, CredentialType: cmd.CredentialType, CeremonyID: in.ceremony.ID, Field: in.ceremony.FieldName, SignedAt: in.signedAt, PDFCID: cid, PDFHash: signedPDFHash, CredentialHashes: credentialHashes}); err != nil {
			return err
		}
	}

	evt := event2.ApplyEvent{
		DID:             cmd.DID,
		ContractVersion: in.contractVersion,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		CredentialType:  cmd.CredentialType,
		AppliedBy:       cmd.AppliedBy,
		OccurredAt:      in.signedAt,
	}
	if err := event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return nil
}

// archiveSignedContract creates the archive entry for a contract that just
// reached SIGNED (DCS-FR-CWE-20: the archive-entry trigger is gated to
// SIGNED, not APPROVED), notarizing and RFC-3161-TSA-timestamping it exactly
// as the former APPROVED-time trigger did.
func (h *Applier) archiveSignedContract(ctx context.Context, tx *sqlx.Tx, did string, appliedBy string, signingEvidence cwecommand.ArchiveSigningEvidence) error {
	signedContract, err := h.ArchiveRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read signed contract for archive storage: %w", err)
	}

	archiveEntry, err := cwecommand.BuildArchiveEntry(signedContract, appliedBy, signingEvidence)
	if err != nil {
		return fmt.Errorf("could not build archive entry: %w", err)
	}
	if h.IPFSStorer == nil {
		return errors.New("archive snapshot IPFS storer is required")
	}
	snapshotCID, err := h.IPFSStorer.Put(ctx, artifactstore.ContractScope(did), []byte(archiveEntry.ContractSnapshot))
	if err != nil {
		return fmt.Errorf("could not store archive snapshot in IPFS: %w", err)
	}
	if snapshotCID == "" {
		return errors.New("archive snapshot IPFS storer returned empty CID")
	}
	archiveEntry.SnapshotCID = snapshotCID

	archiveEntryID := fmt.Sprintf("%s#%d", did, signedContract.ContractVersion)
	notaryPayload := cwecommand.ArchiveNotaryPayload{
		EventType:       "ARCHIVE_STORED",
		ArchiveEntryID:  archiveEntryID,
		DID:             did,
		ContractVersion: signedContract.ContractVersion,
		ContentHash:     archiveEntry.ContentHash,
		SnapshotCID:     archiveEntry.SnapshotCID,
		StoredBy:        appliedBy,
		StoredAt:        archiveEntry.StoredAt,
	}
	var notaryReceipt *cwecommand.ArchiveNotaryReceipt
	if h.ArchiveNotary != nil {
		notaryReceipt, err = h.ArchiveNotary.NotarizeArchiveEntry(ctx, notaryPayload)
		if err != nil {
			return fmt.Errorf("could not notarize archive entry: %w", err)
		}
	}

	var tsaReceipt *cweevent.ArchiveTSAReceipt
	if h.ArchiveTSA != nil && h.ArchiveTSA.Enabled() && notaryReceipt != nil {
		evidence, err := cwecommand.BuildArchiveTimestampEvidence(notaryPayload, notaryReceipt)
		if err != nil {
			return fmt.Errorf("could not build archive TSA evidence: %w", err)
		}
		evidenceBytes, err := cwecommand.CanonicalArchiveTimestampEvidence(evidence)
		if err != nil {
			return err
		}
		rawReceipt, err := h.ArchiveTSA.TimestampBytes(ctx, evidenceBytes)
		if err != nil {
			return fmt.Errorf("could not timestamp archive entry: %w", err)
		}
		tsaReceipt = &cweevent.ArchiveTSAReceipt{
			ReceiptType:    "ARCHIVE_TSA_RECEIPT",
			Token:          rawReceipt.Token,
			TokenEncoding:  rawReceipt.TokenEncoding,
			HashAlgorithm:  rawReceipt.HashAlgorithm,
			MessageImprint: rawReceipt.MessageImprint,
			GeneratedAt:    rawReceipt.GeneratedAt,
			Policy:         rawReceipt.Policy,
			SerialNumber:   rawReceipt.SerialNumber,
		}
		tsaReceiptJSON, err := datatype.NewJSON(tsaReceipt)
		if err != nil {
			return fmt.Errorf("could not encode archive TSA receipt: %w", err)
		}
		archiveEntry.TSAReceipt = &tsaReceiptJSON
	}

	if err := h.ArchiveRepo.StoreArchiveEntry(ctx, tx, archiveEntry); err != nil {
		return fmt.Errorf("could not store contract in archive: %w", err)
	}

	var notaryEventReceipt *cweevent.ArchiveNotaryReceipt
	if notaryReceipt != nil {
		notaryEventReceipt = &cweevent.ArchiveNotaryReceipt{
			ReceiptType:    notaryReceipt.ReceiptType,
			ArchiveEntryID: notaryReceipt.ArchiveEntryID,
			EventHash:      notaryReceipt.EventHash,
			PreviousHash:   notaryReceipt.PreviousHash,
			ReceivedAt:     notaryReceipt.ReceivedAt,
		}
	}
	archiveEvt := cweevent.StoreArchivedEvent{
		DID:             did,
		ContractVersion: signedContract.ContractVersion,
		StoredBy:        appliedBy,
		ContentHash:     archiveEntry.ContentHash,
		SnapshotCID:     archiveEntry.SnapshotCID,
		ArchiveStatus:   "STORED",
		NotaryReceipt:   notaryEventReceipt,
		TSAReceipt:      tsaReceipt,
		EvidenceSummary: cweevent.ArchiveEvidenceSummary{
			SnapshotHashAlgorithm: "SHA-256",
			SignatureStatus:       "SIGNED",
			CredentialHashStatus:  "HASHED",
		},
		OccurredAt: time.Now().UTC(),
	}
	if err := event.Create(ctx, tx, archiveEvt, componenttype.ContractStorageArchive); err != nil {
		return fmt.Errorf("could not create archive store event: %w", err)
	}

	return nil
}

// loadBasePDF returns the PDF the signature covers: the contract's stored
// artifact, or a fresh base render from the JSON-LD when nothing is stored yet.
// A fresh render carries no PAdES signature, so a contract with no stored
// artifact is never frozen and the caller's artifactFrozen verdict — taken from
// the same stored bytes before this — holds for the returned document too.
func (h *Applier) loadBasePDF(ctx context.Context, storedPDF, jsonld []byte) ([]byte, error) {
	if len(storedPDF) > 0 {
		return storedPDF, nil
	}
	pdfBytes, _, err := h.PDFCore.Download(ctx, jsonld)
	if err != nil {
		return nil, fmt.Errorf("render base PDF: %w", err)
	}
	return pdfBytes, nil
}

// stampLifecycleForSigning embeds the "active" C2PA lifecycle assertion
// (DCS-OR-C2PA-004) into pdfBytes and returns the updated PDF plus the
// renderer version pdf-core reports. It is the update-then-sign counterpart of
// pdfgeneration/query.stampLifecycle: called BEFORE PAdES-signing so the
// signature commits to the PDF's final lifecycle-bearing content, and the
// signed artefact never needs a post-signature revision for the SIGNED/ACTIVE
// transition (see the Applier.VCIssuer field doc comment).
func stampLifecycleForSigning(
	ctx context.Context,
	did string,
	jsonldBytes, pdfBytes []byte,
	pdfCore *pdfcore.Client,
	vcIssuer provenance.VCIssuer,
	issuerDID string,
) ([]byte, string, error) {
	const c2paState = "active"
	const reason = "Contract activated for execution"

	h := sha256.Sum256(pdfBytes)
	assetHash := hex.EncodeToString(h[:])

	_, vcBytes, err := vcIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, issuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, "", fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := pdfCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, provenance.RemoteManifestURL(did))
	if err != nil {
		return pdfBytes, "", fmt.Errorf("pdf-core update for %s: %w", did, err)
	}
	return updatedPDF, rendererVersion, nil
}

// contractDocumentForSignature returns the machine-readable contract this
// signature covers, and whether it differs from the stored one.
//
// A signature may only change the contract document while that document can
// still be re-embedded into the PDF the signature covers. Once the artifact
// carries a PAdES signature it can never be re-rendered, so a change here would
// leave contract_data and the document embedded in this instance's own PDF
// permanently apart — and the peer rebuilds the expected JAdES payload from the
// EMBEDDED document (service/dcs_to_dcs.go verifyShippedJades), so every ship of
// that contract is refused with "JAdES payload does not match the contract
// document embedded in the shipped PDF", forever, on a 10-second retry. It is
// the same invariant contractworkflowengine's requireUnsettledAgreement enforces
// against a renegotiation, applied to the signing path it exempts.
//
// On the frozen path the signatory and the Power of Attorney behind this
// signature travel in the signing summary credential instead, embedded as one
// more associated file before the countersignature is applied — the shape the
// receiver already reads them in (dcstodcs/counterpartypoa.go) and the only one
// available to a countersignature.
func contractDocumentForSignature(
	raw datatype.JSON, responsible *db.Responsible, signerDID, poaOrganization, signingParty string, artifactFrozen bool,
) (datatype.JSON, bool, error) {
	if artifactFrozen {
		return raw, false, nil
	}

	// The first signature is the acceptance act: the offered policy set becomes
	// the odrl:Agreement the signatures bind, sealed BEFORE the content hash and
	// the PDF are computed so the signed artefact and the machine-readable
	// document are the same bytes.
	sealed, err := sealAgreementForSigning(raw, responsible, signerDID)
	if err != nil {
		return nil, false, fmt.Errorf("seal agreement for signing: %w", err)
	}
	attributed, err := recordSignatory(sealed, responsible, signerDID, poaOrganization, signingParty)
	if err != nil {
		return nil, false, fmt.Errorf("record signatory: %w", err)
	}
	return attributed, true, nil
}

// sealAgreementForSigning turns the offered policy set into the
// odrl:Agreement the signatures bind: the enclosing policy node retypes,
// and a still-open role-derived party placeholder is rewritten to the
// accepting counterparty's identity — the one workflow peer distinct from
// the originator when there is exactly one, otherwise the signer's
// verified DID — with the signing identity recorded as dcs:hasSignatory.
// Binding only happens while exactly one placeholder remains open, so an
// undeclared originator role never gets mislabeled as the counterparty.
func sealAgreementForSigning(raw datatype.JSON, responsible *db.Responsible, signerDID string) (datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode contract data: %w", err)
	}

	if policies, ok := doc["dcs:policies"].(map[string]any); ok {
		policies["@type"] = "odrl:Agreement"
	}

	// The offeror is the contracting party (ODRL §4.3.7 — "the Party who is
	// offering the contract"); the accepting counterparty is the contracted
	// party (§4.3.8). Both are signatories.
	if responsible != nil && responsible.Creator != "" {
		if node := partyNodeByID(doc, responsible.Creator); node != nil {
			node["odrl:function"] = map[string]any{"@id": "odrl:contractingParty"}
		}
	}

	if placeholder := singleOpenPartyPlaceholder(doc); placeholder != "" {
		counterparty := counterpartyIdentity(responsible, signerDID)
		replaceNodeIRI(doc, placeholder, counterparty)
		mergePartyNodes(doc, counterparty)
		if node := partyNodeByID(doc, counterparty); node != nil {
			node["odrl:function"] = map[string]any{"@id": "odrl:contractedParty"}
		}
	}

	return datatype.NewJSON(doc)
}

// recordSignatory attributes one applied signature: who signed, for which
// party, and under what authority. It runs for every signature, where sealing
// runs once.
//
// The signatory belongs on the node of the party that SIGNED. Stamping it on
// the counterparty's node coincides with the signer only when the accepting
// counterparty signs first; when the originator does — which every two-instance
// flow drives — it records the originator's signatory against the OTHER party,
// and the Power of Attorney with it. A peer verifying the evidence behind that
// signature then looks up the party the credential authorizes and finds a node
// carrying neither.
//
// Auto-seeded signature fields are named for the signing instance's DID, so the
// field name IS the party. An authored multi-signatory contract names its fields
// freely and such a name identifies no party node; the fallback then attributes
// the signature to the accepting counterparty, which is where it landed before.
// That is not right — it is the same misattribution this fixes for the
// two-instance case — but naming the signing party for a freely-named field
// needs the field-to-party mapping the document does not carry.
func recordSignatory(raw datatype.JSON, responsible *db.Responsible, signerDID, poaOrganization, signingParty string) (datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode contract data: %w", err)
	}

	node := signingPartyNode(doc, responsible, signerDID, signingParty)
	if node == nil {
		return raw, nil
	}
	node["dcs:hasSignatory"] = map[string]any{"@id": signerDID}
	// The organization the signatory presented a Power of Attorney for at
	// signing (UC-14, FR-SM-03); it travels with the contract to peers so a
	// counterparty's authorization is auditable on every instance.
	if poaOrganization != "" {
		node["dcs:hasPowerOfAttorney"] = map[string]any{"@id": poaOrganization}
	}

	return datatype.NewJSON(doc)
}

func signingPartyNode(doc map[string]any, responsible *db.Responsible, signerDID, signingParty string) map[string]any {
	if party := strings.TrimSpace(signingParty); party != "" {
		if node := partyNodeByID(doc, party); node != nil {
			return node
		}
	}
	return partyNodeByID(doc, counterpartyIdentity(responsible, signerDID))
}

// counterpartyIdentity resolves who accepted the offer: the contract's
// counterparty peer (ADR-13), or the verified signer when the workflow ran on
// a single instance with no counterparty.
func counterpartyIdentity(responsible *db.Responsible, signerDID string) string {
	if responsible == nil || responsible.Counterparty == "" {
		return signerDID
	}
	return responsible.Counterparty
}

// singleOpenPartyPlaceholder returns the IRI of the only dcs:parties node
// still carrying a role-derived #party-<role> placeholder ("" when none or
// several remain).
func singleOpenPartyPlaceholder(doc map[string]any) string {
	nodes, _ := doc["dcs:parties"].([]any)
	open := []string{}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		iri, _ := node["@id"].(string)
		if _, role, found := strings.Cut(iri, "#party-"); found {
			if _, isIndexed := strconvAtoiOK(role); !isIndexed {
				open = append(open, iri)
			}
		}
	}
	if len(open) == 1 {
		return open[0]
	}
	return ""
}

// strconvAtoiOK reports whether s is a plain index (an attachContractParties
// read-authorization node, never a role placeholder).
func strconvAtoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// mergePartyNodes folds every dcs:parties node carrying id into the first of
// them and drops the rest. Binding a role placeholder to a party the document
// already declares (the contract's counterparty, seeded at creation) leaves two
// nodes under one IRI, and everything downstream reads the first one — so the
// role the placeholder carried would be lost, or found, depending on order.
// Properties already on the surviving node win.
func mergePartyNodes(doc map[string]any, id string) {
	nodes, _ := doc["dcs:parties"].([]any)
	var kept map[string]any
	merged := make([]any, 0, len(nodes))
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			merged = append(merged, rawNode)
			continue
		}
		if iri, _ := node["@id"].(string); iri != id {
			merged = append(merged, rawNode)
			continue
		}
		if kept == nil {
			kept = node
			merged = append(merged, rawNode)
			continue
		}
		for key, value := range node {
			if _, present := kept[key]; !present {
				kept[key] = value
			}
		}
	}
	doc["dcs:parties"] = merged
}

func partyNodeByID(doc map[string]any, id string) map[string]any {
	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		if node, ok := rawNode.(map[string]any); ok {
			if iri, _ := node["@id"].(string); iri == id {
				return node
			}
		}
	}
	return nil
}

// replaceNodeIRI rewrites every "@id" equal to old with new, recursively.
func replaceNodeIRI(current any, old, new string) {
	switch value := current.(type) {
	case map[string]any:
		if iri, _ := value["@id"].(string); iri == old {
			value["@id"] = new
		}
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	case []any:
		for _, nested := range value {
			replaceNodeIRI(nested, old, new)
		}
	}
}

// partyDIDs lists the contract's party DIDs for contractstate.IsRemotePartyField,
// and nothing at all for a contract that records no parties.
func partyDIDs(resp *db.Responsible) []string {
	if resp == nil {
		return nil
	}
	return []string{resp.Creator, resp.Counterparty}
}

// derefStr returns "" for a nil string pointer.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// oidGivenName/oidSurname are the X.520 RDN attribute types (eIDAS Annex I
// natural-person certificate fields) — crypto/x509/pkix.Name has no
// dedicated GivenName/Surname fields (unlike CommonName), so they must be
// read from the certificate's raw parsed RDN sequence.
var (
	oidGivenName = asn1.ObjectIdentifier{2, 5, 4, 42}
	oidSurname   = asn1.ObjectIdentifier{2, 5, 4, 4}
)

// signerCertificateFromIncrementalUpdate extracts the X.509 certificate that
// produced the signature signedPDF's most recent incremental update carries —
// exactly the bytes this submission itself added on top of preparedPDF
// (already proven to be an exact byte-prefix of signedPDF, ADR-20 byte
// pinning) — from that update's own CMS SignerInfo, independent of anything
// DSS's validation report chooses to expose.
func signerCertificateFromIncrementalUpdate(signedPDF, preparedPDF []byte) (*x509.Certificate, error) {
	if len(signedPDF) < len(preparedPDF) {
		return nil, fmt.Errorf("signed PDF is shorter than the prepared document")
	}
	delta := signedPDF[len(preparedPDF):]
	der, err := extractContentsDER(delta)
	if err != nil {
		return nil, fmt.Errorf("locate the newly-added signature's /Contents: %w", err)
	}
	p7, err := pkcs7.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("parse the newly-added signature's CMS SignerInfo: %w", err)
	}
	cert := p7.GetOnlySigner()
	if cert == nil {
		return nil, fmt.Errorf("the newly-added signature's CMS carries no matching signer certificate")
	}
	return cert, nil
}

// extractContentsDER locates the first PDF hex-string /Contents value in raw
// and decodes it. PAdES signature dictionaries are never placed inside a
// compressed object stream — the byte-range signing mechanism requires them
// to be byte-addressable in the plain revision — so a direct search is
// reliable, the same direct-byte-search approach this repo already uses
// elsewhere for CMS/COSE contents.
func extractContentsDER(raw []byte) ([]byte, error) {
	marker := []byte("/Contents")
	idx := bytes.Index(raw, marker)
	if idx < 0 {
		return nil, fmt.Errorf("no /Contents entry found")
	}
	rest := bytes.TrimLeft(raw[idx+len(marker):], " \t\r\n")
	if len(rest) == 0 || rest[0] != '<' {
		return nil, fmt.Errorf("/Contents is not a hex string")
	}
	end := bytes.IndexByte(rest, '>')
	if end < 0 {
		return nil, fmt.Errorf("/Contents hex string has no closing '>'")
	}
	// PDF hex strings may contain whitespace between digit pairs; the
	// trailing run of zero bytes padding the /Contents placeholder to its
	// pre-allocated length decodes fine and is ignored by ASN.1 parsing,
	// which stops at the DER SEQUENCE's own declared length.
	hexDigits := bytes.Join(bytes.Fields(rest[1:end]), nil)
	der := make([]byte, hex.DecodedLen(len(hexDigits)))
	n, err := hex.Decode(der, hexDigits)
	if err != nil {
		return nil, fmt.Errorf("decode /Contents hex: %w", err)
	}
	return der[:n], nil
}

// certGivenSurname reads the GIVENNAME/SURNAME RDN attributes from a
// certificate's subject.
func certGivenSurname(cert *x509.Certificate) (given, surname string) {
	for _, atv := range cert.Subject.Names {
		s, ok := atv.Value.(string)
		if !ok {
			continue
		}
		switch {
		case atv.Type.Equal(oidGivenName):
			given = s
		case atv.Type.Equal(oidSurname):
			surname = s
		}
	}
	return given, surname
}

// pidGivenFamilyName extracts given_name/family_name from the ceremony's
// stored PID claims (the German EUDI PID's standard claim names; camelCase is
// accepted as a fallback for other issuer conventions).
func pidGivenFamilyName(pidClaims []byte) (given, family string) {
	if len(pidClaims) == 0 {
		return "", ""
	}
	var claims map[string]any
	if err := json.Unmarshal(pidClaims, &claims); err != nil {
		return "", ""
	}
	given, _ = claims["given_name"].(string)
	if given == "" {
		given, _ = claims["givenName"].(string)
	}
	family, _ = claims["family_name"].(string)
	if family == "" {
		family, _ = claims["familyName"].(string)
	}
	return strings.TrimSpace(given), strings.TrimSpace(family)
}

// normalizeName folds a name for comparison: case-insensitive, whitespace-
// collapsed. Certificates and PID claims are not guaranteed to agree on
// capitalization or spacing even when they name the same person.
func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.ToUpper(s)), " ")
}

// namesMatch reports whether the PID's given/family name and the
// certificate's given name/surname name the same person (sole control,
// ADR-20). Both PID fields and at least one certificate field must be
// present — an empty certificate subject is never treated as a match by
// omission.
func namesMatch(pidGiven, pidFamily, certGiven, certSurname string) bool {
	if pidGiven == "" || pidFamily == "" {
		return false
	}
	if certGiven == "" && certSurname == "" {
		return false
	}
	return normalizeName(pidGiven) == normalizeName(certGiven) && normalizeName(pidFamily) == normalizeName(certSurname)
}

// jwsPayloadBytes base64url-decodes the payload segment of a compact JWS
// (JAdES). The signature has already been validated by the time this is
// called, so the header and payload are known-authentic.
func jwsPayloadBytes(compact string) ([]byte, error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected a compact JWS with three segments")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}

// jwsProtectedHeaderClaim reads a string claim from a compact JWS's protected
// header. Used to recover the nonce a wallet embeds in its JAdES protected
// header to bind the signature to the ceremony's request nonce (ADR-20) — the
// header is covered by the JWS signature, so this is only trustworthy AFTER
// the signature has validated.
func jwsProtectedHeaderClaim(compact, claim string) (string, error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("expected a compact JWS with three segments")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode protected header: %w", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return "", fmt.Errorf("parse protected header: %w", err)
	}
	value, _ := header[claim].(string)
	return value, nil
}
