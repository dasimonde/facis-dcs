// Package event subscribes to CWE lifecycle state-change events and appends
// a new C2PA manifest to the contract's stored PDF for each transition
// (DCS-OR-C2PA-001, DCS-OR-C2PA-003, DCS-OR-C2PA-008).
package event

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cweevent "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tplevttype "digital-contracting-service/internal/templaterepository/datatype/eventtype"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

// contractLifecycleEventTypes is the set of CWE event types that change a
// contract's rendered content or lifecycle state and therefore require the PDF
// to be regenerated in the background — including Update (a content edit), so
// the exported PDF is never generated on demand.
var contractLifecycleEventTypes = map[string]bool{
	cweeventtype.Create.String():                  true,
	cweeventtype.Update.String():                  true,
	cweeventtype.Offer.String():                   true,
	cweeventtype.Withdraw.String():                true,
	cweeventtype.Submit.String():                  true,
	cweeventtype.Approve.String():                 true,
	cweeventtype.Reject.String():                  true,
	cweeventtype.Terminate.String():               true,
	cweeventtype.ContractExpired.String():         true,
	cweeventtype.Negotiation.String():             true,
	cweeventtype.IncreaseContractVersion.String(): true,
}

// templateLifecycleEventTypes is the set of template repository event types
// that change a template's content or state and require background PDF
// regeneration — including Update (a content edit).
var templateLifecycleEventTypes = map[string]bool{
	tplevttype.Create.String():   true,
	tplevttype.Update.String():   true,
	tplevttype.Submit.String():   true,
	tplevttype.Approve.String():  true,
	tplevttype.Reject.String():   true,
	tplevttype.Verify.String():   true,
	tplevttype.Archive.String():  true,
	tplevttype.Register.String(): true,
}

// minimalCWEEvent extracts common fields present in all CWE event structs.
type minimalCWEEvent struct {
	DID        string    `json:"did"`
	NewState   string    `json:"new_state"`
	OccurredAt time.Time `json:"occurred_at"`
	Reason     string    `json:"reason,omitempty"`
}

// Subscriber listens to the NATS event bus and appends C2PA lifecycle
// assertions to the PDF stored in IPFS for each CWE state-change event.
type Subscriber struct {
	DB        *sqlx.DB
	Artifacts *artifactstore.Store
	CRepo     cwedb.ContractRepo
	TRepo     tpldb.ContractTemplateRepo
	PDFCore   *pdfcore.Client
	IssuerDID string
	// LocalPeer is this instance's own did:web. A contract whose Origin is not
	// this DID was received from a peer (ADR-13); its stored PDF carries the
	// counterparty's C2PA chain, so a content change must amend that base rather
	// than fresh-render (which would strip the counterparty's provenance).
	LocalPeer string
	// VCIssuer issues and signs a W3C VC for each lifecycle event (DCS-OR-C2PA-004/005).
	VCIssuer provenance.VCIssuer
	// retries bounds and paces the retry sweep's attempts per entity.
	retries retryBudget
}

// regenerationTimeout bounds one regeneration attempt: a pdf-core render, a VC
// issuance and an artifact-store write.
const regenerationTimeout = 60 * time.Second

// regenerationRetryBatch bounds how many entities one retry pass regenerates, so
// a deployment with a large backlog makes steady progress per tick instead of
// occupying the regenerator indefinitely.
const regenerationRetryBatch = 25

// regenerationRetryAttempts bounds how often the sweep re-attempts one entity,
// and maxRegenerationRetryBackoff caps the wait between those attempts. The
// batch is a fixed-size window over the oldest rows: an entity whose
// regeneration can never succeed would otherwise be selected on every tick
// forever and starve every recoverable failure behind it.
const (
	regenerationRetryAttempts   = 5
	maxRegenerationRetryBackoff = time.Hour
)

// Start registers the event handler with the NATS sub-client and begins
// consuming events, alongside the retry pass that reconciles entities whose
// regeneration never completed. It returns immediately; both run in the
// background until the sub-client is closed.
func (s *Subscriber) Start(subClient *event.CloudEventSubClient) error {
	go s.retryPendingRegenerations(conf.PDFRegenerationRetryTimeOut())

	return subClient.Subscribe(func(evt cloudevent.Event) {
		ctx, cancel := s.regenerationContext()
		defer cancel()
		if err := s.handle(ctx, evt); err != nil {
			log.Printf("pdfgeneration: failed to handle event %s/%s: %v", evt.Source(), evt.Type(), err)
		}
	})
}

// regenerationContext is the context every regeneration attempt runs under. It
// carries no credential: pdf-core holds no key and never calls back, so the
// regenerator reaches nothing that authenticates it.
func (s *Subscriber) regenerationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), regenerationTimeout)
}

// retryPendingRegenerations re-runs regeneration for entities whose stored PDF
// is not a render of the document as it now stands. A lifecycle event is
// delivered at most once and a failed regeneration is logged rather than
// redelivered, so a transient pdf-core or artifact-store failure would
// otherwise leave the entity stuck for good — not exportable, and, for a
// cross-instance contract, not shippable, so the counterparty never receives it
// however often the sync-fail scheduler retries the ship. That holds whether
// the lost regeneration was the entity's FIRST (leaving no stored PDF at all)
// or a LATER one (leaving a stored PDF the document has moved past, which the
// ship refuses just as flatly). Both belong on the work list, and both come
// from the entity's own committed state rather than a failure record, so the
// pass also recovers regenerations lost to a restart. Every handler
// short-circuits when the PDF is already current, so a redundant selection
// costs two reads.
func (s *Subscriber) retryPendingRegenerations(interval time.Duration) {
	s.retries.pace(interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		contracts, templates, err := s.entitiesNeedingRegeneration()
		if err != nil {
			log.Printf("pdfgeneration: could not read entities needing regeneration: %v", err)
			continue
		}
		now := time.Now()
		for _, did := range contracts {
			if !s.retries.ready("contract", did, now) {
				continue
			}
			s.retryOne("contract", did, s.appendC2PA)
		}
		for _, did := range templates {
			if !s.retries.ready("template", did, now) {
				continue
			}
			s.retryOne("template", did, s.appendTemplateC2PA)
		}
	}
}

// retryOne regenerates one entity. The event that first requested the
// regeneration is gone, so the attempt carries no reason and is effective now;
// everything else the handlers need they re-read from the record.
func (s *Subscriber) retryOne(kind, did string, regenerate func(context.Context, minimalCWEEvent) error) {
	ctx, cancel := s.regenerationContext()
	defer cancel()
	if err := regenerate(ctx, minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC()}); err != nil {
		attempts := s.retries.failed(kind, did, time.Now())
		log.Printf("pdfgeneration: retry %d/%d for %s %s did not regenerate the PDF: %v",
			attempts, regenerationRetryAttempts, kind, did, err)
		return
	}
	s.retries.succeeded(kind, did)
}

// carriesSignature reports whether the contract holds a committed signature —
// the thing that makes its stored PDF unrenderable.
func (s *Subscriber) carriesSignature(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	count, err := s.CRepo.CountSignedSignatures(ctx, tx, did)
	if err != nil {
		return false, fmt.Errorf("count signatures on contract %s: %w", did, err)
	}
	return count > 0, nil
}

func (s *Subscriber) entitiesNeedingRegeneration() ([]string, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contracts, err := s.CRepo.ReadDIDsNeedingRegeneration(ctx, tx, regenerationRetryBatch,
		s.retries.exhausted("contract"))
	if err != nil {
		return nil, nil, fmt.Errorf("read contracts whose stored PDF is not current: %w", err)
	}
	templates, err := s.TRepo.ReadDIDsMissingStoredPDF(ctx, tx, regenerationRetryBatch,
		s.retries.exhausted("template"))
	if err != nil {
		return nil, nil, fmt.Errorf("read templates without a stored PDF: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit read: %w", err)
	}
	return contracts, templates, nil
}

func (s *Subscriber) handle(ctx context.Context, evt cloudevent.Event) error {
	isContract := contractLifecycleEventTypes[evt.Type()]
	isTemplate := templateLifecycleEventTypes[evt.Type()]
	if !isContract && !isTemplate {
		return nil
	}

	// The outbox publisher passes the domain event straight through as
	// json.RawMessage (cloudeventprovider.go: marshalling a RawMessage is the
	// identity), so the CloudEvent data IS the domain event object.
	var cweEvt minimalCWEEvent
	if err := json.Unmarshal(evt.Data(), &cweEvt); err != nil {
		return fmt.Errorf("unmarshal CWE event: %w", err)
	}
	if cweEvt.DID == "" {
		return nil // non-entity events (e.g. RetrieveAll) have no DID
	}

	if isTemplate {
		return s.appendTemplateC2PA(ctx, cweEvt)
	}
	return s.appendC2PA(ctx, cweEvt)
}

// frozenArtifactVerdict decides what a background regeneration may do to a
// contract whose PDF is frozen — a PAdES-signed artifact (DCS-FR-SM-16), or a
// peer's signed bytes received verbatim. The signing command already produced
// and stored the final bytes, and any post-signing C2PA lifecycle update runs
// through the explicit signing/revoke endpoints, never this regenerator:
// re-rendering here would replace the signed PDF with an unsigned one and
// destroy the signature's /ByteRange. Returns true when there is nothing to do.
//
// What freezes an artifact is a SIGNATURE, not a state past "draft". The stored
// artifact's own C2PA state says so directly — it was written by the signing
// command. The target state does not: a contract reaches a frozen state without
// ever being signed (DRAFT -> APPROVED -> terminate, or the expiry cron flipping
// an unsigned contract to EXPIRED), and freezing on it would decline the FIRST
// render into that state, leaving pdf_state behind the contract forever — an
// export that polls to its deadline and answers "being regenerated" for good.
// So the target state freezes only together with a committed signature, which
// covers the case the stored artifact cannot: a contract signed HERE whose
// stored CID is empty (the artifact store accepted the signed bytes but the
// pointer never committed) reads as "not frozen" on the artifact alone, and the
// regeneration path's answer to a missing artifact is a FRESH render — an
// unsigned PDF, issued a new lifecycle VC. There is nothing to render from in
// that case; the artifact can only come back from its signed bytes.
//
// A peer's signature is invisible to that count — CountSignedSignatures reads
// only this instance's contract_signatures, and the receive path writes none.
// What protects a peer's signed bytes is the stored artifact's own state, which
// receivepdf records from the shipped PDF (provenance.ArtifactC2PAState) rather
// than from this instance's workflow state.
//
// carriesSignature is consulted only when the answer can still change the
// verdict, so an ordinary draft regeneration costs no extra query.
func frozenArtifactVerdict(did, contractState, storedCID, storedC2PAState, targetC2PAState string,
	carriesSignature func() (bool, error)) (bool, error) {
	frozen := provenance.IsFrozenC2PAState(storedC2PAState)
	if !frozen && provenance.IsFrozenC2PAState(targetC2PAState) {
		signed, err := carriesSignature()
		if err != nil {
			return false, err
		}
		frozen = signed
	}
	if !frozen {
		return false, nil
	}
	if storedCID == "" {
		return false, fmt.Errorf(
			"contract %s carries a signature (state %q) but holds no stored PDF: refusing to render one, a signed contract's artifact can only be restored from its signed bytes",
			did, contractState)
	}
	return true, nil
}

func (s *Subscriber) appendC2PA(ctx context.Context, cweEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Serialize regeneration per contract IRI: concurrent lifecycle events for
	// the same contract queue on this lock instead of racing the read-modify-
	// write of the PDF state (which would double-render and could fork the C2PA
	// chain). Released on tx commit/rollback.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", cweEvt.DID); err != nil {
		return fmt.Errorf("acquire per-contract PDF regeneration lock for %s: %w", cweEvt.DID, err)
	}

	// Fetch current contract state and JSON-LD.
	contract, err := s.CRepo.ReadDataByDID(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read contract %s: %w", cweEvt.DID, err)
	}

	var jsonldBytes []byte
	if contract.ContractData != nil {
		jsonldBytes = []byte(*contract.ContractData)
	}
	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	// Map the contract's committed state to the SRS-defined C2PA vocabulary
	// (DCS-OR-C2PA-003). The record's state is the source of truth: the genesis
	// CreateEvent carries no new_state, and the event is emitted only after the
	// transition commits, so the record always reflects the state the PDF must
	// assert.
	c2paState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	pdfState, err := s.CRepo.ReadPDFState(ctx, tx, cweEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for contract %s: %w", cweEvt.DID, err)
	}

	frozen, err := frozenArtifactVerdict(cweEvt.DID, string(contract.State), pdfState.IPFSCID, pdfState.C2PAState, c2paState,
		func() (bool, error) { return s.carriesSignature(ctx, tx, cweEvt.DID) })
	if err != nil {
		return err
	}
	if frozen {
		return nil
	}

	contentChanged := pdfState.PayloadHash != currentPayloadHash
	stateChanged := pdfState.C2PAState != c2paState
	if pdfState.IPFSCID != "" && !contentChanged && !stateChanged {
		return nil // already up to date — idempotent re-delivery
	}

	// The signature fields are seeded at genesis (create.go), so the initial
	// render already carries the full signable AcroForm structure and no later
	// render needs to introduce fields. Every regeneration therefore AMENDS the
	// stored PDF (pdfCore.Update chains the prior manifest as an ingredient) —
	// whether the change is a state transition, a local content edit, or a
	// peer-received counter-offer — so the C2PA provenance chain and any embedded
	// signatures always carry through and grow instead of resetting (ADR-13). The
	// inbound peer PDF is the authoritative base: it holds provenance and
	// credentials this instance cannot reproduce. A fresh render happens only at
	// genesis, when there is no stored PDF yet.
	var basePDF []byte
	if pdfState.IPFSCID != "" {
		basePDF, err = s.Artifacts.Get(ctx, artifactstore.ContractScope(cweEvt.DID), pdfState.IPFSCID)
		if err != nil || len(basePDF) == 0 {
			return fmt.Errorf("fetch PDF from IPFS %s for contract %s: %w", pdfState.IPFSCID, cweEvt.DID, err)
		}
	} else {
		basePDF, _, err = s.PDFCore.Download(ctx, jsonldBytes)
		if err != nil {
			return fmt.Errorf("pdf-core render for contract %s: %w", cweEvt.DID, err)
		}
	}

	// Compute asset hash for the VC credentialSubject (DCS-OR-C2PA-004).
	h := sha256.Sum256(basePDF)
	fileHash := hex.EncodeToString(h[:])

	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, cweEvt.DID, fileHash, c2paState, cweEvt.Reason, s.IssuerDID, cweEvt.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, basePDF, jsonldBytes, vcBytes, provenance.RemoteManifestURL(cweEvt.DID))
	if err != nil {
		return fmt.Errorf("pdf-core update for contract %s: %w", cweEvt.DID, err)
	}

	storedCID, err := s.Artifacts.Put(ctx, artifactstore.ContractScope(cweEvt.DID), updatedPDF)
	if err != nil {
		return fmt.Errorf("store updated PDF in IPFS for contract %s: %w", cweEvt.DID, err)
	}

	if err = s.CRepo.UpdatePDFState(ctx, tx, cweEvt.DID, cwedb.ContractPDFState{IPFSCID: storedCID, RendererVersion: rendererVersion, C2PAState: c2paState, PayloadHash: currentPayloadHash}); err != nil {
		return fmt.Errorf("update pdf_ipfs_cid for %s: %w", cweEvt.DID, err)
	}

	if err := event.Create(ctx, tx, cweevent.PdfRegeneratedEvent{
		DID:        cweEvt.DID,
		IPFSCID:    storedCID,
		State:      string(contract.State),
		OccurredAt: time.Now().UTC(),
	}, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("emit PDF-regenerated event for %s: %w", cweEvt.DID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pdf_ipfs_cid update for %s: %w", cweEvt.DID, err)
	}

	log.Printf("pdfgeneration: regenerated PDF for contract %s (state=%s, contentChanged=%t) → IPFS CID %s", cweEvt.DID, contract.State, contentChanged, storedCID)
	return nil
}

// appendTemplateC2PA appends a C2PA lifecycle assertion to a contract template's
// stored PDF in response to a template state-change event (DCS-OR-C2PA-003).
func (s *Subscriber) appendTemplateC2PA(ctx context.Context, tplEvt minimalCWEEvent) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := s.TRepo.ReadDataByID(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tplEvt.DID, err)
	}

	var jsonldBytes []byte
	if tpl.TemplateData != nil {
		jsonldBytes = []byte(*tpl.TemplateData)
	}

	// The template record's state is the source of truth (the genesis CreateEvent
	// carries no new_state); the event is emitted only after the transition commits.
	c2paState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
	}

	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	tplPDFState, err := s.TRepo.ReadPDFState(ctx, tx, tplEvt.DID)
	if err != nil {
		return fmt.Errorf("read PDF state for template %s: %w", tplEvt.DID, err)
	}

	contentChanged := tplPDFState.PayloadHash != currentPayloadHash
	stateChanged := tplPDFState.C2PAState != c2paState
	if tplPDFState.IPFSCID != "" && !contentChanged && !stateChanged {
		return nil // already up to date
	}

	// State transition appends to preserve the chain; genesis or a content edit
	// renders fresh from the current content.
	var pdfBytes []byte
	if tplPDFState.IPFSCID != "" && !contentChanged {
		pdfBytes, err = s.Artifacts.Get(ctx, artifactstore.TemplateScope(tplEvt.DID), tplPDFState.IPFSCID)
		if err != nil || len(pdfBytes) == 0 {
			return fmt.Errorf("fetch PDF from IPFS %s for template %s: %w", tplPDFState.IPFSCID, tplEvt.DID, err)
		}
	} else {
		pdfBytes, _, err = s.PDFCore.Download(ctx, jsonldBytes)
		if err != nil {
			return fmt.Errorf("pdf-core download for template %s: %w", tplEvt.DID, err)
		}
	}

	pdfBytes, err = s.appendOneTemplateManifest(ctx, tx, tplEvt.DID, tpl.State, jsonldBytes, pdfBytes, tplEvt.OccurredAt)
	if err != nil {
		return fmt.Errorf("append C2PA manifest for template %s: %w", tplEvt.DID, err)
	}
	_ = pdfBytes // result stored in IPFS and DB inside appendOneTemplateManifest

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit C2PA update for template %s: %w", tplEvt.DID, err)
	}

	log.Printf("pdfgeneration: regenerated PDF for template %s (state=%s, contentChanged=%t)", tplEvt.DID, tpl.State, contentChanged)
	return nil
}

// appendOneTemplateManifest issues a VC, calls pdf-core /update, stores the result
// in IPFS, and updates pdf_ipfs_cid / pdf_c2pa_state in contract_templates within tx.
// It returns the updated PDF bytes.
func (s *Subscriber) appendOneTemplateManifest(
	ctx context.Context, tx *sqlx.Tx,
	did, state string, jsonldBytes, pdfBytes []byte, effectiveAt time.Time,
) ([]byte, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", state, err)
	}

	h := sha256.Sum256(pdfBytes)
	fileHash := hex.EncodeToString(h[:])
	payloadHashSum := sha256.Sum256(jsonldBytes)
	currentPayloadHash := hex.EncodeToString(payloadHashSum[:])

	_, vcBytes, err := s.VCIssuer.IssueContractLifecycleVC(
		ctx, did, fileHash, c2paState, "", s.IssuerDID, effectiveAt,
	)
	if err != nil {
		return nil, fmt.Errorf("issue lifecycle VC: %w", err)
	}

	// pdf-core appends C2PA incremental update with VC attachment.
	// vcBytes being non-nil bypasses the "no-changes" guard for genesis VC attachment.
	// Templates have no public /c2pa/manifest/{contract_did} endpoint, so no
	// remote_manifests reference is embedded for the template PDF path.
	updatedPDF, rendererVersion, err := s.PDFCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, "")
	if err != nil {
		return nil, fmt.Errorf("pdf-core update for template %s: %w", did, err)
	}

	storedCID, err := s.Artifacts.Put(ctx, artifactstore.TemplateScope(did), updatedPDF)
	if err != nil {
		return nil, fmt.Errorf("store updated PDF in IPFS for template %s: %w", did, err)
	}

	if err := s.TRepo.UpdatePDFState(ctx, tx, did, tpldb.ContractTemplatePDFState{IPFSCID: storedCID, RendererVersion: rendererVersion, C2PAState: c2paState, PayloadHash: currentPayloadHash}); err != nil {
		return nil, fmt.Errorf("update contract_templates pdf_ipfs_cid: %w", err)
	}

	return updatedPDF, nil
}
