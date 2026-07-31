package dcstodcs

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
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

// AuditedShredder is the production ScopeShredder: it marks every wrapped-CEK
// record of the contract scope destroyed and writes the KEY_SHREDDED audit
// event in the same transaction, then drops the plaintext CEK from the store
// cache. Repeating a shred is a no-op without a duplicate event.
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

// DBContractParties reads the contract's parties from its Responsible record.
type DBContractParties struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
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
	return data.Responsible.GetParties(), nil
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
// KEY_SHREDDED audit event, then ask every counterparty instance to shred its
// own via /peer/contracts/erase. An unreachable peer leaves a pending
// contract_erasures row that the retry scheduler re-delivers.
type ContractEraser struct {
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
// row, failure counts a retry attempt.
func (e *ContractEraser) attemptPeerErase(ctx context.Context, contractIRI, peer string) error {
	localPeer, err := e.DIDDocument.GetID()
	if err != nil {
		return err
	}
	secretValue := rand.Text()
	secretHash, err := e.DIDDocument.Sign([]byte(secretValue))
	if err != nil {
		return err
	}
	sendErr := e.Sender.SendErase(ctx, peer, &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: localPeer,
		ContractIri: contractIRI,
		SecretValue: secretValue,
		SecretHash:  secretHash,
	})
	if sendErr != nil {
		if err := e.ERepo.RecordAttempt(ctx, contractIRI, peer); err != nil {
			log.Printf(ctx, "could not record erase attempt for %s toward %s: %v", contractIRI, peer, err)
		}
		return sendErr
	}
	return e.ERepo.Confirm(ctx, contractIRI, peer)
}

// RetryPending re-delivers every not-yet-confirmed peer erase request.
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
// (same mechanics as the sync-fail scheduler, separate queue).
func (e *ContractEraser) StartEraseRetryJob(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			e.RetryPending(ctx)
		}
	}()
}
