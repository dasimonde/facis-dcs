package dcstodcs

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/datatype/componenttype"
	basedb "digital-contracting-service/internal/base/db"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
)

// TerminationNoticeBody is the body the challenge of a termination notice
// binds: the reason. Whoever relays the notice cannot change it.
func TerminationNoticeBody(reason *string) []byte {
	if reason == nil {
		return nil
	}
	return []byte(*reason)
}

// PeerTerminationSender delivers one termination notice to a counterparty
// instance.
type PeerTerminationSender interface {
	SendTermination(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSContractTerminateRequest) error
}

// HTTPPeerTerminationSender calls POST /peer/contracts/terminate on the peer
// resolved from its did:web identifier.
type HTTPPeerTerminationSender struct{}

func (HTTPPeerTerminationSender) SendTermination(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSContractTerminateRequest) error {
	hostname, segments, err := identity.DIDWebPath(peerDID)
	if err != nil {
		return err
	}
	peerPrefix := ""
	if len(segments) > 0 {
		peerPrefix = "/" + strings.Join(segments, "/")
	}
	_, err = NewDCSToDCSHttpClient(hostname, peerPrefix).Terminate(ctx, req)
	return err
}

// TerminationNotifier delivers the pending rows of contract_peer_terminations
// (DCS-NFR-BR-06). The terminate command writes a row per counterparty that
// holds a copy, in the transaction that terminates the contract. A row stays
// pending until the peer confirms, or until the peer refuses it for good
// because it holds no copy shared with this instance.
type TerminationNotifier struct {
	DB          *sqlx.DB
	DIDDocument identity.DIDDocument
	Sender      PeerTerminationSender
}

type pendingTermination struct {
	DID     string `db:"did"`
	PeerDID string `db:"peer_did"`
	Reason  string `db:"reason"`
}

// DeliverPending sends every unconfirmed notice that is due. A failed notice
// waits five seconds longer per attempt, up to five minutes.
func (n *TerminationNotifier) DeliverPending(ctx context.Context) {
	var pending []pendingTermination
	if err := n.DB.SelectContext(ctx, &pending,
		`SELECT did, peer_did, reason FROM contract_peer_terminations
		  WHERE confirmed_at IS NULL AND refused_at IS NULL
		    AND (last_tried_at IS NULL OR last_tried_at < now() - LEAST(retry_count, 60) * INTERVAL '5 seconds')
		  ORDER BY requested_at`); err != nil {
		log.Printf(ctx, "could not read pending peer terminations: %v", err)
		return
	}
	localPeer, err := n.DIDDocument.GetID()
	if err != nil {
		log.Printf(ctx, "could not resolve the local DID for peer terminations: %v", err)
		return
	}
	for _, notice := range pending {
		secretValue, secretHash, err := SignPeerChallenge(&n.DIDDocument, OpTerminate, localPeer, notice.PeerDID, notice.DID, TerminationNoticeBody(&notice.Reason))
		if err == nil {
			err = n.Sender.SendTermination(ctx, notice.PeerDID, &dcstodcs.DCSToDCSContractTerminateRequest{
				FromPeerDid: localPeer,
				ContractIri: notice.DID,
				Reason:      &notice.Reason,
				SecretValue: secretValue,
				SecretHash:  secretHash,
			})
		}
		switch {
		case IsFinalPeerRefusal(err):
			log.Printf(ctx, "termination notice for %s toward %s refused for good: %v", notice.DID, notice.PeerDID, err)
			if dbErr := n.recordRefusal(ctx, notice, err); dbErr != nil {
				log.Printf(ctx, "could not record the refused termination notice for %s toward %s: %v", notice.DID, notice.PeerDID, dbErr)
			}
		case err != nil:
			log.Printf(ctx, "termination notice for %s toward %s not delivered, retry pending: %v", notice.DID, notice.PeerDID, err)
			if _, dbErr := n.DB.ExecContext(ctx, `UPDATE contract_peer_terminations SET retry_count = retry_count + 1, last_tried_at = now(), last_error = $3 WHERE did = $1 AND peer_did = $2`,
				notice.DID, notice.PeerDID, err.Error()); dbErr != nil {
				log.Printf(ctx, "could not record termination attempt for %s toward %s: %v", notice.DID, notice.PeerDID, dbErr)
			}
		default:
			if err := n.recordConfirmation(ctx, notice); err != nil {
				log.Printf(ctx, "could not confirm termination notice for %s toward %s: %v", notice.DID, notice.PeerDID, err)
			}
		}
	}
}

// recordConfirmation confirms the notice and audits the delivered exchange
// (DCS-NFR-BR-08).
func (n *TerminationNotifier) recordConfirmation(ctx context.Context, notice pendingTermination) error {
	tx, err := n.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE contract_peer_terminations SET confirmed_at = now(), last_error = NULL WHERE did = $1 AND peer_did = $2`,
		notice.DID, notice.PeerDID); err != nil {
		return err
	}
	if err := RecordPeerExchangeTx(ctx, tx, notice.DID, "terminate", ExchangeSent, notice.PeerDID); err != nil {
		return err
	}
	return tx.Commit()
}

// recordRefusal ends the notice and records the refusal in the contract's
// lifecycle, so the contract shows that the counterparty's copy was not
// terminated.
func (n *TerminationNotifier) recordRefusal(ctx context.Context, notice pendingTermination, refusal error) error {
	tx, err := n.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE contract_peer_terminations SET last_tried_at = now(), refused_at = now(), last_error = $3 WHERE did = $1 AND peer_did = $2`,
		notice.DID, notice.PeerDID, refusal.Error()); err != nil {
		return err
	}
	if err := baseevent.Create(ctx, tx, PeerExchangeRefusedEvent{
		DID: notice.DID, Exchange: "terminate", PeerDID: notice.PeerDID, Reason: refusal.Error(), OccurredAt: time.Now().UTC(),
	}, componenttype.ContractWorkflowEngine); err != nil {
		return err
	}
	return tx.Commit()
}

// Start delivers pending notices on the given interval.
func (n *TerminationNotifier) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := basedb.RunExclusive(ctx, n.DB, "peer-termination-notices", func() error { n.DeliverPending(ctx); return nil }); err != nil {
					log.Printf(ctx, "could not deliver pending termination notices: %v", err)
				}
			}
		}
	}()
}
