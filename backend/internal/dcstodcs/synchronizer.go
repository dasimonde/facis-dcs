// Package dcstodcs runs the DCS-to-DCS federation on the sending side: it
// listens for the PDF-regenerated event (and the signature-applied event),
// and ships the contract's PDF — the self-contained wire format carrying the
// machine-readable JSON-LD, the C2PA provenance chain, and any signatures — to
// the counterparty peer, gating every ship on the federation trust gate
// (trustgate.go: agreement credential + local policy endpoint, ADR-19) and
// retrying failed ships from the sync_fails table. A signed contract
// additionally carries the JAdES (DCS-FR-SM-02). No contract state or task
// ledger crosses the boundary: each DCS runs its own workflow/RBAC (ADR-13).
package dcstodcs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/semantichub"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type DCSToDCSSynchronizer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	SRepo       db2.SyncRepository
	Artifacts   *artifactstore.Store
	DIDDocument identity.DIDDocument
	TrustGate   TrustGate
	// PoAs supplies the Power of Attorney behind each signature this instance
	// applied, shipped so the counterparty can verify the authority to sign
	// rather than read an unbacked claim off the contract (ADR-31).
	PoAs SignatoryPoAs
	// SettlementSender delivers this instance's settlement artifact — the
	// evidence the counterparty needs before it may sign.
	SettlementSender SettlementSender
}

// shippableStates are the contract states whose PDF is shipped to the
// counterparty (ADR-13): a first offer, each negotiation counter, the signed
// agreement, and a revocation of the applied signature (DCS-NFR-BR-06 —
// revocation must propagate to the counterparty immediately). Internal states
// (DRAFT, SUBMITTED, REVIEWED, APPROVED, ACTIVE, TERMINATED) stay local —
// review/approval never cross the boundary.
var shippableStates = map[string]bool{
	contractstate.Offered.String():     true,
	contractstate.Negotiation.String(): true,
	contractstate.Signed.String():      true,
	contractstate.Revoked.String():     true,
}

func (s *DCSToDCSSynchronizer) StartSynchronizerJob(ctx context.Context, client *event.CloudEventSubClient) {
	syncHandler := func(evt cloudevent.Event) {
		source, err := componenttype.NewComponentType(evt.Source())
		if err != nil {
			log.Errorf(ctx, err, "failed to parse source component type, %s", evt.Source())
			return
		}

		// A PDF is shipped when the regenerator has produced a fresh one
		// (PDF_REGENERATED, content changes: offer/negotiate) or when a
		// signature has been applied (APPLIED_SIGNATURE, which stores the
		// signed PDF directly). shipContractPDF gates on the shippable state.
		switch source {
		case componenttype.ContractWorkflowEngine:
			// The two edges that take an agreement back — a reviewer sending the
			// submission back (SUBMIT_CONTRACT) and an approver rejecting
			// (REJECT_CONTRACT) — queue a withdrawal toward every peer that was
			// told this instance settled. Delivering it here rather than only at
			// the next scheduler tick closes the window in which the peer's
			// signing gate still reads the withdrawn agreement as evidence.
			if evt.Type() == eventtype.Submit.String() || evt.Type() == eventtype.Reject.String() {
				if withdrawnDID, err := didFromEvent(evt); err == nil {
					s.shipSettlementWithdrawals(ctx, withdrawnDID)
				} else {
					log.Errorf(ctx, err, "could not read did from event %s", evt.Data())
				}
			}
			// Settlement (NEGOTIATION -> SUBMITTED) is a statement about BOTH
			// parties — this instance agreed the document — so the counterparty
			// must hold verified evidence of it before it may sign. No PDF and no
			// state cross the boundary here (ADR-13): the artifact binds the
			// document by digest. SUBMITTED is also reached from REVIEWED (an
			// approver sending it back for more review), which settles nothing,
			// so the previous state is what distinguishes the two.
			if evt.Type() == eventtype.Submit.String() {
				settledDID, settled, err := settlementFromEvent(evt)
				if err != nil {
					log.Errorf(ctx, err, "could not read submit event %s", evt.Data())
					return
				}
				if settled {
					if err := s.shipSettlement(ctx, settledDID); err != nil {
						log.Errorf(ctx, err, "failed to ship contract settlement, %s", evt.Data())
					}
				}
				return
			}
			// Ship when the regenerator produced a fresh PDF (a content or C2PA
			// state change) OR when the contract entered the shippable OFFERED
			// state. An offer is a pure state transition that changes neither the
			// payload hash nor the C2PA lifecycle (DRAFT and OFFERED both map to
			// "draft"), so a contract edited while DRAFT and then offered emits no
			// PDF_REGENERATED — the offer itself must trigger the ship of the
			// already-stored PDF (shipContractPDF still gates on the shippable
			// state, so a non-shippable transition is a no-op).
			if evt.Type() != eventtype.PDFRegenerated.String() && evt.Type() != eventtype.Offer.String() {
				return
			}
		case componenttype.SignatureManagement:
			// APPLIED_SIGNATURE ships the signed PDF; REVOKE_SIGNATURE ships
			// the revocation (the contract is in REVOKED by the time the
			// outbox publishes the event, so shipContractPDF reads and
			// declares that state — DCS-NFR-BR-06).
			smType, err := smeventtype.NewEventType(evt.Type())
			if err != nil || (smType != smeventtype.Applied && smType != smeventtype.Revoke) {
				return
			}
		default:
			return
		}

		did, err := didFromEvent(evt)
		if err != nil {
			log.Errorf(ctx, err, "could not read did from event %s", evt.Data())
			return
		}
		if err := s.shipContractPDF(ctx, did); err != nil {
			log.Errorf(ctx, err, "failed to ship contract PDF, %s", evt.Data())
		}
	}

	go func() {
		if err := client.Subscribe(syncHandler); err != nil {
			log.Errorf(ctx, err, "could not start syncHandler")
		}
	}()

	go s.startSyncFailScheduler(ctx, conf.SyncFailCronJobTimeOut())
}

// settlementFromEvent reports which contract a SUBMIT_CONTRACT event settled.
// Only NEGOTIATION -> SUBMITTED is a settlement: it is the transition that
// says every negotiator accepted the document as it stands.
func settlementFromEvent(evt cloudevent.Event) (string, bool, error) {
	var submitted contractevents.SubmitEvent
	if err := json.Unmarshal(evt.Data(), &submitted); err != nil {
		return "", false, fmt.Errorf("unmarshal submit event: %w", err)
	}
	if submitted.DID == "" {
		return "", false, errors.New("submit event carries no did")
	}
	settled := submitted.PreviousState == contractstate.Negotiation.String() &&
		submitted.NewState == contractstate.Submitted.String()
	return submitted.DID, settled, nil
}

func didFromEvent(evt cloudevent.Event) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(evt.Data(), &data); err != nil {
		return "", fmt.Errorf("unmarshal event data: %w", err)
	}
	did, ok := data["did"].(string)
	if !ok || did == "" {
		return "", errors.New("event carries no did")
	}
	return did, nil
}

func (s *DCSToDCSSynchronizer) startSyncFailScheduler(ctx context.Context, interval time.Duration) {
	readSyncFails := func() ([]db2.SyncFail, error) {
		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf(ctx, "could not rollback transaction: %v", err)
			}
		}(tx)

		attempts, err := s.SRepo.GetPendingSyncFails(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to read sync fail entries: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}
		return attempts, nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		log.Printf(ctx, "start retrying failed contract PDF ships")
		syncFails, err := readSyncFails()
		if err != nil {
			log.Printf(ctx, "could not read sync fails: %v", err)
		}
		for _, syncFail := range syncFails {
			if err := s.shipContractPDF(ctx, syncFail.DID); err != nil {
				log.Printf(ctx, "contract PDF ship retry was not successful: %v", err)
			}
		}
		// Settlements and their withdrawals have their own queues: sync_fails is
		// keyed by contract and its retry re-ships the PDF, which would never
		// deliver either.
		s.retryUndeliveredSettlements(ctx)
		s.retryUndeliveredSettlementWithdrawals(ctx)
	}
}

// shipContractPDF fetches the contract's current PDF and ships it to every
// counterparty peer (ADR-13). A signed contract additionally carries the
// JAdES. A failed ship is recorded in sync_fails for later retry; a clean ship
// clears any prior failure. Non-shippable states and contracts without a PDF
// yet are no-ops.
func (s *DCSToDCSSynchronizer) shipContractPDF(ctx context.Context, did string) error {
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return err
	}

	readTx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	contractData, err := s.CRepo.ReadDataByDID(ctx, readTx, did)
	if err != nil {
		_ = readTx.Rollback()
		return fmt.Errorf("could not read contract %s: %w", did, err)
	}
	pdfState, err := s.CRepo.ReadPDFState(ctx, readTx, did)
	if err != nil {
		_ = readTx.Rollback()
		return fmt.Errorf("could not read PDF state for %s: %w", did, err)
	}
	_ = readTx.Rollback()

	state := string(contractData.State)
	if !shippableStates[state] {
		return nil
	}
	if pdfState.IPFSCID == "" {
		// The contract is shippable but its PDF has not been stored yet: the
		// regenerator compiles it asynchronously, so an offer (a pure state
		// transition) can fire before the CID exists. Never silently drop the
		// ship — record a sync_fail so the DB-backed retry scheduler re-attempts
		// once the CID is written. A dropped ship with no record and no retry is
		// a correctness bug, not merely a timing race.
		return s.recordShipOutcome(ctx, did,
			fmt.Errorf("contract %s is shippable but its PDF is not stored yet; deferring ship to the retry scheduler", did), nil)
	}
	// The receiver adopts the JSON-LD carried in the shipped PDF as its own copy
	// (ADR-13), so the PDF must be a render of the document as it now stands. A
	// contract filled and offered while its genesis render is still in flight
	// stores that render afterwards, and the render's own PDF-regenerated event
	// would ship the pre-fill document. Defer to the retry scheduler as for a
	// PDF not stored yet: the re-render's own ship carries the current document.
	if holdsSupersededPDF(pdfState, contractData) {
		return s.recordShipOutcome(ctx, did,
			fmt.Errorf("contract %s holds a PDF rendered from a superseded document; deferring ship until it is re-rendered", did), nil)
	}

	recipients := contractData.Responsible.GetParties()

	pdfBytes, err := s.Artifacts.Get(ctx, artifactstore.ContractScope(did), pdfState.IPFSCID)
	if err != nil || len(pdfBytes) == 0 {
		return fmt.Errorf("fetch PDF %s from IPFS for %s: %w", pdfState.IPFSCID, did, err)
	}

	jadesSignature, err := s.jadesForSignedContract(state, contractData)
	if err != nil {
		return err
	}

	signatoryPoAs, err := s.poaEvidenceForSignedContract(ctx, state, did)
	if err != nil {
		return err
	}

	pinnedShapes, err := s.pinnedShapesForContract(ctx, contractData)
	if err != nil {
		// Unlike the reads above, this fails for stable data-dependent reasons —
		// a pin naming a graph neither namespace resolves — so the event that
		// triggered this ship is the only attempt unless the failure is
		// recorded. Route it through the retry queue like the two deferrals
		// above: a dropped ship with no record and no retry is a correctness
		// bug, not merely a timing race.
		return s.recordShipOutcome(ctx, did, err, nil)
	}

	shipError := s.shipToPeers(ctx, localPeer, did, state, pdfBytes, jadesSignature, signatoryPoAs, pinnedShapes, recipients)

	var gateErr *GateError
	if errors.As(shipError, &gateErr) && gateErr.Kind == PolicyFailure {
		// Unified terminal semantics (ADR-19): a policy-endpoint denial —
		// non-2xx, unset DCS_TRUST_PDP_URL, or unreachable — never gets a
		// sync_fails retry entry, so every attempt is a genuinely new
		// interaction, not a retry of the same denial: exactly one incident
		// per attempt (contrast the agreement-credential dedup below). Any
		// sync_fails row a PRIOR, unrelated failure left behind (e.g. the
		// "PDF not stored yet" deferral, which runs before the trust gate is
		// even consulted) must be cleared here too — otherwise the terminal
		// PDP denial would still sit in the retry queue and keep getting
		// re-attempted forever. Cleared atomically with the incident so a
		// crash between the two can't leave one without the other.
		if err := s.clearSyncFailWithIncident(ctx, did, gateErr); err != nil {
			log.Printf(ctx, "could not clear sync fail entry / record trust gate denial incident for %s: %v", did, err)
		}
		return nil
	}
	return s.recordShipOutcome(ctx, did, shipError, gateErr)
}

// clearSyncFailWithIncident deletes any sync_fails retry entry for did and
// records the terminal policy-endpoint denial incident in the same
// transaction (ADR-19 makes every trust-gate denial auditable through the
// incident-report flow) — deduped per (did, peer, direction), since a
// single offer's Offer and PDF_REGENERATED events each independently trigger
// a ship attempt a few hundred ms apart and both can hit the same denial.
func (s *DCSToDCSSynchronizer) clearSyncFailWithIncident(ctx context.Context, did string, gateErr *GateError) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	if err := s.SRepo.DeleteSyncFailEntry(ctx, tx, did); err != nil {
		return fmt.Errorf("could not delete sync fail entry: %w", err)
	}
	if err := RecordDenialIncidentTxDeduped(ctx, tx, s.DB, did, Outbound, gateErr); err != nil {
		return fmt.Errorf("could not record trust gate denial incident: %w", err)
	}
	return tx.Commit()
}

// holdsSupersededPDF reports whether the stored PDF was rendered from a
// document the contract has since moved past, which is what makes it unfit to
// ship. A frozen artifact answers false — signed bytes are never re-rendered,
// so their recorded hash is the one signing wrote, not a render's. So does a
// contract whose PDF predates payload-hash recording: nothing is known about
// what it was rendered from, and refusing to ship it would strand it.
func holdsSupersededPDF(pdfState *db.ContractPDFState, contract *db.Contract) bool {
	if pdfState == nil || provenance.IsFrozenC2PAState(pdfState.C2PAState) || pdfState.PayloadHash == "" {
		return false
	}
	return pdfState.PayloadHash != contractDataHash(contract)
}

// contractDataHash is the payload hash of a contract's stored document, taken
// exactly as the regenerator takes the one it records against a render
// (pdfgeneration/event.Subscriber.appendC2PA) — the two are only comparable
// while they hash the same bytes the same way.
func contractDataHash(contract *db.Contract) string {
	var payload []byte
	if contract.ContractData != nil {
		payload = []byte(*contract.ContractData)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// jadesForSignedContract returns the JAdES a signed contract's ship carries, or
// an empty string for a proposal ship (ADR-13, DCS-FR-SM-02).
func (s *DCSToDCSSynchronizer) jadesForSignedContract(state string, contractData *db.Contract) (string, error) {
	if state != contractstate.Signed.String() {
		return "", nil
	}
	contractDocBytes := []byte(`{}`)
	if contractData.ContractData != nil && contractData.ContractData.IsNotNullValue() {
		contractDocBytes = []byte(*contractData.ContractData)
	}
	payload, err := jades.BuildContractPayload(contractData.DID, contractData.ContractVersion, contractDocBytes)
	if err != nil {
		return "", fmt.Errorf("build JAdES payload for %s: %w", contractData.DID, err)
	}
	signature, err := jades.Sign(&s.DIDDocument, payload)
	if err != nil {
		return "", fmt.Errorf("JAdES-sign %s: %w", contractData.DID, err)
	}
	return signature, nil
}

// poaEvidenceForSignedContract returns the Power of Attorney behind every
// signature this instance applied, for the ships where a signature exists at
// all (ADR-31). A proposal carries none because nothing has been signed yet.
func (s *DCSToDCSSynchronizer) poaEvidenceForSignedContract(ctx context.Context, state, did string) ([]SignatoryPoA, error) {
	if s.PoAs == nil {
		return nil, nil
	}
	if state != contractstate.Signed.String() && state != contractstate.Revoked.String() {
		return nil, nil
	}
	evidence, err := s.PoAs.ForContract(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("read power-of-attorney evidence for %s: %w", did, err)
	}
	return evidence, nil
}

// pinnedShapesForContract reads the shape libraries the contract pins in
// dcs:effectiveShapes (ADR-8). They ship with every PDF: the pin is what the
// receiver's workflow gate resolves, and a library published on this instance
// alone exists nowhere else, so without them the receiver holds a copy no
// transition can evaluate.
func (s *DCSToDCSSynchronizer) pinnedShapesForContract(ctx context.Context, contractData *db.Contract) ([]semantichub.Schema, error) {
	document := []byte(`{}`)
	if contractData.ContractData != nil && contractData.ContractData.IsNotNullValue() {
		document = []byte(*contractData.ContractData)
	}
	entries, err := PinnedShapesForDocument(ctx, semantichub.DBPinnedShapes{DB: s.DB}, document)
	if err != nil {
		return nil, fmt.Errorf("resolve the pinned shapes of %s: %w", contractData.DID, err)
	}
	return entries, nil
}

func (s *DCSToDCSSynchronizer) shipToPeers(ctx context.Context, localPeer, did, state string, pdfBytes []byte, jadesSignature string, signatoryPoAs []SignatoryPoA, pinnedShapes []semantichub.Schema, recipients []string) error {
	for _, peer := range recipients {
		if peer == localPeer {
			continue
		}
		if err := s.TrustGate.Check(ctx, peer, Outbound, did, state); err != nil {
			return err
		}

		// Ship the contract CEK wrapped to the peer's keyAgreement key
		// (DCS-NFR-SEC-14): the receiver re-wraps it to its own key, so both
		// instances hold the same CEK for the contract. Sent with every ship —
		// the receiver adopts it only when it has none yet, so repeats are
		// idempotent. A peer without a resolvable keyAgreement key cannot
		// receive the CEK and the ship hard-fails into the retry queue.
		//
		// The wrap carries the id of the method it was made for, so a peer that
		// publishes several key-agreement keys knows which one to unwrap with.
		remoteDIDDocument, err := identity.FetchDIDDocument(peer)
		if err != nil {
			return fmt.Errorf("fetch did document of peer %s: %w", peer, err)
		}
		peerKeyAgreement, err := remoteDIDDocument.PeerKeyAgreementMethod()
		if err != nil {
			return fmt.Errorf("resolve keyAgreement key of peer %s: %w", peer, err)
		}
		peerKeyAgreementPub, err := peerKeyAgreement.ECPublicKey()
		if err != nil {
			return fmt.Errorf("keyAgreement key %q of peer %s: %w", peerKeyAgreement.ID, peer, err)
		}
		wrappedCEK, err := s.Artifacts.WrapForPeer(ctx, artifactstore.ContractScope(did), peer, peerKeyAgreement.ID, peerKeyAgreementPub)
		if err != nil {
			return fmt.Errorf("wrap contract cek of %s for peer %s: %w", did, peer, err)
		}

		hostname, segments, err := identity.DIDWebPath(peer)
		if err != nil {
			return err
		}
		peerPrefix := ""
		if len(segments) > 0 {
			peerPrefix = "/" + strings.Join(segments, "/")
		}
		secretValue := rand.Text()
		secretHash, err := s.DIDDocument.Sign([]byte(secretValue))
		if err != nil {
			return err
		}
		client := NewDCSToDCSHttpClient(hostname, peerPrefix)
		if _, err := client.PostPdf(ctx, &dcstodcs.DCSToDCSContractPdfRequest{
			FromPeerDid:    localPeer,
			ContractIri:    did,
			Pdf:            pdfBytes,
			SecretValue:    secretValue,
			SecretHash:     secretHash,
			JadesSignature: &jadesSignature,
			ContractState:  &state,
			WrappedCek:     WireWrappedCEK(wrappedCEK),
			SignatoryPoas:  WireSignatoryPoAs(signatoryPoAs),
			PinnedShapes:   WirePinnedShapes(pinnedShapes),
		}); err != nil {
			return err
		}
	}
	return nil
}

// recordShipOutcome persists a ship attempt's sync_fails side effect and,
// for an agreement-credential trust-gate failure (gateErr != nil), records
// exactly one incident for it. The sync_fails row this contract's ship
// attempts share can be created for an UNRELATED reason first (e.g. the PDF
// not being stored yet, gateErr == nil that time) and only turn into an
// actual gate failure on a later retry — "was this call the one that
// inserted the row" is therefore NOT the right dedup signal (it would miss
// the incident entirely in that interleaving); CreateOrUpdateSyncFailEntry's
// own gate_incident_recorded latch is, and it reports true at most once
// per entry regardless of how many times gate failures do or don't recur.
func (s *DCSToDCSSynchronizer) recordShipOutcome(ctx context.Context, did string, shipError error, gateErr *GateError) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	shouldRecordIncident := false
	if shipError != nil {
		shouldRecordIncident, err = s.SRepo.CreateOrUpdateSyncFailEntry(ctx, tx, did, gateErr != nil)
		if err != nil {
			return fmt.Errorf("could not create or update sync fail entry: %w", err)
		}
	} else if err := s.SRepo.DeleteSyncFailEntry(ctx, tx, did); err != nil {
		return fmt.Errorf("could not delete sync fail entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if shouldRecordIncident && gateErr != nil {
		if incidentErr := RecordDenialIncident(ctx, s.DB, did, Outbound, gateErr); incidentErr != nil {
			log.Printf(ctx, "could not record trust gate denial incident for %s: %v", did, incidentErr)
		}
	}
	return shipError
}
