package dcstodcs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	basedb "digital-contracting-service/internal/base/db"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"goa.design/clue/log"
	goa "goa.design/goa/v3/pkg"
)

// ScopeShredder destroys a contract scope's wrapped CEK records and leaves
// the KEY_SHREDDED destruction record behind (DCS-NFR-SEC-13).
type ScopeShredder interface {
	Shred(ctx context.Context, contractIRI, actor, reason string) (int64, error)
}

// ContractParties resolves the contract's party DIDs (creator plus
// counterparties), the recipients of a peer erase.
type ContractParties interface {
	Parties(ctx context.Context, contractIRI string) ([]string, error)
}

// PeerEraseSender delivers one erase request to a counterparty instance.
type PeerEraseSender interface {
	SendErase(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSContractEraseRequest) error
}

// AuditedShredder is the production ScopeShredder: it destroys every
// wrapped-CEK record of the contract scope, erases the plaintext contract
// content from Postgres and writes the KEY_SHREDDED audit event in the same
// transaction, then drops the plaintext CEK from the store cache. Repeating a
// shred is a no-op without a duplicate event.
type AuditedShredder struct {
	DB        *sqlx.DB
	Keys      *artifactstore.PostgresCEKRepo
	Artifacts *artifactstore.Store
}

func (s *AuditedShredder) Shred(ctx context.Context, contractIRI, actor, reason string) (int64, error) {
	scope := artifactstore.ContractScope(contractIRI)

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	affected, err := s.Keys.ShredTx(ctx, tx, scope, actor, reason)
	if err != nil {
		return 0, fmt.Errorf("shred cek records for %s: %w", contractIRI, err)
	}
	if err := eraseContractContent(ctx, tx, contractIRI); err != nil {
		return 0, fmt.Errorf("erase plaintext content of %s: %w", contractIRI, err)
	}
	if affected > 0 {
		evt := contractevents.KeyShreddedEvent{
			DID:          contractIRI,
			ScopeKind:    string(scope.Kind),
			ScopeID:      scope.ID,
			ShreddedBy:   actor,
			Reason:       reason,
			RowsShredded: affected,
			OccurredAt:   time.Now().UTC(),
		}
		if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
			return 0, fmt.Errorf("record KEY_SHREDDED event for %s: %w", contractIRI, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.Artifacts.Forget(scope)
	return affected, nil
}

// eraseContractContent removes the plaintext copies of the contract content
// and of the signers' personal data that Postgres holds (DCS-NFR-SEC-13,
// DCS-NFR-COMP-03): the current and historical contract data, name and
// description, negotiation change requests, rejection reasons and drafts,
// the termination reasons, the archive snapshot, the payload pinned for
// signing, the PDF prepared for signing, the PID claims and presentations of
// the signing ceremonies, the payload of every JAdES signature over the
// contract, and the outbox copies of the event bodies that the audit trail
// erases with the key. The contract keeps its party names for the
// read policy. Metadata, hashes and signature values stay as evidence.
func eraseContractContent(ctx context.Context, tx *sqlx.Tx, contractIRI string) error {
	statements := []string{
		`UPDATE contracts SET contract_data = ` + partyNamesOnly + ` WHERE did = $1 AND contract_data IS DISTINCT FROM ` + partyNamesOnly,
		`UPDATE contracts SET name = NULL, description = NULL WHERE did = $1 AND num_nonnulls(name, description) > 0`,
		`UPDATE contract_history SET contract_data = '{}'::jsonb WHERE did = $1 AND contract_data IS DISTINCT FROM '{}'::jsonb`,
		`UPDATE contract_history SET name = NULL, description = NULL WHERE did = $1 AND num_nonnulls(name, description) > 0`,
		`UPDATE contract_negotiations SET change_request = '{}'::jsonb WHERE did = $1 AND change_request IS DISTINCT FROM '{}'::jsonb`,
		`UPDATE contract_negotiation_decisions SET rejection_reason = NULL
		 WHERE rejection_reason IS NOT NULL AND negotiation_id IN (SELECT id FROM contract_negotiations WHERE did = $1)`,
		// A termination always records a reason (contract_terminations_reason_present),
		// so an erased one reads as the erased marker.
		`UPDATE contract_terminations SET reason = '` + ErasedReason + `' WHERE did = $1 AND reason <> '` + ErasedReason + `'`,
		`UPDATE contract_peer_terminations SET reason = '` + ErasedReason + `' WHERE did = $1 AND reason <> '` + ErasedReason + `'`,
		`DELETE FROM contract_negotiation_drafts WHERE contract_did = $1`,
		`UPDATE contract_archive_entries SET contract_snapshot = '{}'::jsonb WHERE did = $1 AND contract_snapshot <> '{}'::jsonb`,
		`UPDATE signature_ceremonies SET pinned_payload = NULL, prepared_pdf = NULL, pid_claims = NULL, vp_token = NULL, poa_vp_token = NULL
		 WHERE contract_did = $1 AND num_nonnulls(pinned_payload, prepared_pdf, pid_claims, vp_token, poa_vp_token) > 0`,
		`UPDATE contract_sync_signatures SET jades_signature = ` + detachedJWS + ` WHERE did = $1 AND split_part(jades_signature, '.', 2) <> ''`,
		`UPDATE contract_signatures SET jades_signature = ` + detachedJWS + ` WHERE contract_did = $1 AND split_part(jades_signature, '.', 2) <> ''`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement, contractIRI); err != nil {
			return err
		}
	}
	return eraseOutboxBodies(ctx, tx, contractIRI)
}

// ErasedReason replaces an erased termination reason.
const ErasedReason = "[erased]"

// partyNamesOnly keeps the dcs:legalName of each dcs:parties node: the
// organization names contractreadpolicy authorizes on. Party nodes carry no
// other field after the erasure.
const partyNamesOnly = `(SELECT CASE WHEN jsonb_typeof(contract_data->'dcs:parties') = 'array'
	THEN jsonb_build_object('dcs:parties', COALESCE((
		SELECT jsonb_agg(jsonb_build_object('dcs:legalName', party->'dcs:legalName'))
		FROM jsonb_array_elements(contract_data->'dcs:parties') party
		WHERE jsonb_typeof(party->'dcs:legalName') = 'string'), '[]'::jsonb))
	ELSE '{}'::jsonb END)`

// detachedJWS removes the payload from a compact JWS (RFC 7515 Appendix F):
// the protected header and the signature value stay as evidence.
const detachedJWS = `split_part(jades_signature, '.', 1) || '..' || split_part(jades_signature, '.', 3)`

// eraseOutboxBodies replaces the outbox copy of every event body that the
// audit trail encrypts under the contract's key with the erased marker. The
// audit chain hashes the stored IPFS entry, not the outbox row, so the chain
// and the checkpoint proofs keep verifying. The erasure records and the
// events of other scopes keep their bodies, as they do in the audit trail.
func eraseOutboxBodies(ctx context.Context, tx *sqlx.Tx, contractIRI string) error {
	var events []datatype.OutboxEvent
	if err := tx.SelectContext(ctx, &events,
		`SELECT id, component, event_type, did FROM outbox_events WHERE did = $1`, contractIRI); err != nil {
		return err
	}
	contractScope := artifactstore.ContractScope(contractIRI)
	ids := make([]int64, 0, len(events))
	for _, evt := range events {
		if baseevent.ScopeForEvent(evt, artifactstore.Scope{}) == contractScope {
			ids = append(ids, evt.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE outbox_events SET event_data = $1 WHERE id = ANY($2)`,
		string(datatype.ErasedEventData), pq.Array(ids))
	return err
}

// DBContractParties reads the parties of the contract that hold a copy of it:
// the origin this instance received it from, and each peer this instance
// delivered a revision to. A party that never received the contract gets no
// erase request.
type DBContractParties struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
	SRepo db2.SyncRepository
}

func (p *DBContractParties) Parties(ctx context.Context, contractIRI string) ([]string, error) {
	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	data, err := p.CRepo.ReadDataByDID(ctx, tx, contractIRI)
	if err != nil {
		return nil, fmt.Errorf("could not read contract %s: %w", contractIRI, err)
	}
	if data.Responsible == nil {
		return nil, nil
	}
	deliveries, err := p.SRepo.GetPDFDeliveries(ctx, tx, contractIRI)
	if err != nil {
		return nil, fmt.Errorf("could not read the deliveries of contract %s: %w", contractIRI, err)
	}
	return db2.PeersHoldingCopy(data.Responsible.GetParties(), data.Origin, deliveries), nil
}

// The refusals of PeerErasure.AuthorizeErase.
var (
	// ErrNoSharedCopy: this instance holds no copy of the contract that the
	// requesting peer is a party of. The refusal does not change.
	ErrNoSharedCopy = errors.New("this instance holds no copy of the contract shared with the requesting peer")
	// ErrNotErasable: the copy is in force, or a retention period of its
	// archive entries runs. The refusal can change later.
	ErrNotErasable = errors.New("the local copy of the contract may not be erased now")
)

// PeerErasure decides whether a peer may have the local copy of a contract
// erased.
type PeerErasure interface {
	AuthorizeErase(ctx context.Context, contractIRI, peerDID string) error
}

// DBPeerErasure allows a peer erase only for a party of the contract, only
// for a copy that is no longer in force, and only while no live archive entry
// of the contract is under retention.
type DBPeerErasure struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (e DBPeerErasure) AuthorizeErase(ctx context.Context, contractIRI, peerDID string) error {
	tx, err := e.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exists, err := e.CRepo.ExistsByDID(ctx, tx, contractIRI)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: no contract %s", ErrNoSharedCopy, contractIRI)
	}
	contract, err := e.CRepo.ReadDataByDID(ctx, tx, contractIRI)
	if err != nil {
		return err
	}
	if contract.Responsible == nil || !slices.Contains(contract.Responsible.GetParties(), peerDID) {
		return fmt.Errorf("%w: peer %s is not a party of contract %s", ErrNoSharedCopy, peerDID, contractIRI)
	}
	// The local archive delete uses the same predicate (DCS-FR-CSA-17).
	if !contractstate.ContractState(contract.State).Erasable() {
		return fmt.Errorf("%w: contract %s is %s, and only a TERMINATED, EXPIRED, REVOKED, REJECTED or WITHDRAWN contract may be erased",
			ErrNotErasable, contractIRI, contract.State)
	}
	var retainedUntil *time.Time
	if err := tx.GetContext(ctx, &retainedUntil, `
		SELECT max(retention_until) FROM contract_archive_entries
		 WHERE did = $1 AND deleted_at IS NULL AND retention_until > now()`, contractIRI); err != nil {
		return err
	}
	if retainedUntil != nil {
		return fmt.Errorf("%w: the archive entries of contract %s are retained until %s",
			ErrNotErasable, contractIRI, retainedUntil.UTC().Format(time.RFC3339))
	}
	return nil
}

// HTTPPeerEraseSender calls POST /peer/contracts/erase on the peer resolved
// from its did:web identifier.
type HTTPPeerEraseSender struct{}

func (HTTPPeerEraseSender) SendErase(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSContractEraseRequest) error {
	hostname, segments, err := identity.DIDWebPath(peerDID)
	if err != nil {
		return err
	}
	peerPrefix := ""
	if len(segments) > 0 {
		peerPrefix = "/" + strings.Join(segments, "/")
	}
	client := NewDCSToDCSHttpClient(hostname, peerPrefix)
	_, err = client.Erase(ctx, req)
	return err
}

// ContractEraser runs the erasure of a contract's content-encryption keys
// (DCS-NFR-COMP-03, DCS-NFR-SEC-13): shred the local wrapped CEKs with a
// KEY_SHREDDED audit event, then ask every counterparty instance that holds a
// copy to shred its own via /peer/contracts/erase. An unreachable peer, or a
// peer whose copy is still in force or under retention, leaves a pending
// contract_erasures row that the retry scheduler re-delivers with backoff. A
// peer that holds no copy shared with this instance refuses for good: the row
// is marked refused and is not sent again.
type ContractEraser struct {
	DB          *sqlx.DB
	DIDDocument identity.DIDDocument
	Shredder    ScopeShredder
	Parties     ContractParties
	ERepo       db2.EraseRepository
	Sender      PeerEraseSender
}

// EraseContract shreds the contract's local CEK records and triggers the peer
// erase handshake toward every counterparty. The local shred is a hard error;
// an undeliverable peer request is not — it stays pending for retry. Purely
// local contracts shred locally only.
func (e *ContractEraser) EraseContract(ctx context.Context, contractIRI, actor, reason string) error {
	// Party resolution comes first: nothing is destroyed if the contract
	// cannot be read.
	parties, err := e.Parties.Parties(ctx, contractIRI)
	if err != nil {
		return err
	}
	localPeer, err := e.DIDDocument.GetID()
	if err != nil {
		return err
	}

	if _, err := e.Shredder.Shred(ctx, contractIRI, actor, reason); err != nil {
		return err
	}

	for _, peer := range parties {
		if peer == localPeer {
			continue
		}
		if err := e.ERepo.Upsert(ctx, contractIRI, peer); err != nil {
			return fmt.Errorf("record peer erase request for %s toward %s: %w", contractIRI, peer, err)
		}
		if err := e.attemptPeerErase(ctx, contractIRI, peer); err != nil {
			log.Printf(ctx, "peer erase for %s toward %s not delivered, retry pending: %v", contractIRI, peer, err)
		}
	}
	return nil
}

// attemptPeerErase delivers one erase request; success confirms the ledger
// row, a final refusal marks it refused, any other failure counts a retry
// attempt.
func (e *ContractEraser) attemptPeerErase(ctx context.Context, contractIRI, peer string) error {
	localPeer, err := e.DIDDocument.GetID()
	if err != nil {
		return err
	}
	secretValue, secretHash, err := SignPeerChallenge(&e.DIDDocument, OpErase, localPeer, peer, contractIRI, nil)
	if err != nil {
		return err
	}
	sendErr := e.Sender.SendErase(ctx, peer, &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: localPeer,
		ContractIri: contractIRI,
		SecretValue: secretValue,
		SecretHash:  secretHash,
	})
	switch {
	case IsFinalPeerRefusal(sendErr):
		if err := e.ERepo.Refuse(ctx, contractIRI, peer, sendErr.Error()); err != nil {
			log.Printf(ctx, "could not record the refused erase of %s toward %s: %v", contractIRI, peer, err)
		}
		return sendErr
	case sendErr != nil:
		if err := e.ERepo.RecordAttempt(ctx, contractIRI, peer, sendErr.Error()); err != nil {
			log.Printf(ctx, "could not record erase attempt for %s toward %s: %v", contractIRI, peer, err)
		}
		return sendErr
	}
	if err := e.ERepo.Confirm(ctx, contractIRI, peer); err != nil {
		return err
	}
	if err := RecordPeerExchange(ctx, e.DB, contractIRI, "erase", ExchangeSent, peer); err != nil {
		log.Printf(ctx, "could not audit the erase of %s toward %s: %v", contractIRI, peer, err)
	}
	return nil
}

// IsFinalPeerRefusal reports whether a peer refused a request for a reason
// that does not change: it holds no copy of the contract shared with this
// instance.
func IsFinalPeerRefusal(err error) bool {
	var serviceErr *goa.ServiceError
	return errors.As(err, &serviceErr) && serviceErr.Name == "not_found"
}

// RetryPending re-delivers the peer erase requests that are neither confirmed
// nor refused and whose backoff has passed.
func (e *ContractEraser) RetryPending(ctx context.Context) {
	pending, err := e.ERepo.GetPending(ctx)
	if err != nil {
		log.Printf(ctx, "could not read pending peer erasures: %v", err)
		return
	}
	for _, request := range pending {
		if err := e.attemptPeerErase(ctx, request.DID, request.PeerDID); err != nil {
			log.Printf(ctx, "peer erase retry for %s toward %s was not successful: %v", request.DID, request.PeerDID, err)
		}
	}
}

// StartEraseRetryJob re-delivers pending peer erasures on the given interval
// (same mechanics as the sync-fail scheduler, separate queue), on one replica
// at a time.
func (e *ContractEraser) StartEraseRetryJob(ctx context.Context, database *sqlx.DB, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			if _, err := basedb.RunExclusive(ctx, database, "peer-erase-retry", func() error { e.RetryPending(ctx); return nil }); err != nil {
				log.Printf(ctx, "could not retry pending peer erasures: %v", err)
			}
		}
	}()
}
