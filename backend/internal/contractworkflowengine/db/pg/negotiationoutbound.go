package pg

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresNegotiationOutboundRepo struct{}

func (r PostgresNegotiationOutboundRepo) Enqueue(ctx context.Context, tx *sqlx.Tx, msg db.NegotiationOutboundMessage) (string, error) {
	switch msg.Kind {
	case db.NegotiationMessageProposal:
		return r.enqueueProposal(ctx, tx, msg)
	case db.NegotiationMessageWithdraw:
		return r.enqueueWithdraw(ctx, tx, msg)
	case db.NegotiationMessageAccept, db.NegotiationMessageReject:
		return r.enqueueDecision(ctx, tx, msg)
	case db.NegotiationMessageOfferAccepted:
		return r.enqueueOfferAccepted(ctx, tx, msg)
	default:
		return "", fmt.Errorf("unknown negotiation outbound kind %q", msg.Kind)
	}
}

func (r PostgresNegotiationOutboundRepo) enqueueOfferAccepted(ctx context.Context, tx *sqlx.Tx, msg db.NegotiationOutboundMessage) (string, error) {
	negotiationID := msg.NegotiationID
	if negotiationID == "" {
		negotiationID = db.OfferAcceptedSentinelNegotiationID
	}
	statement := `
        INSERT INTO contract_negotiation_outbound (
            did, from_peer_did, to_peer_did, kind, negotiation_id, decision_id, contract_version
        ) VALUES ($1, $2, $3, 'OFFER_ACCEPTED'::contract_negotiation_message_kind, $4, NULL, $5)
        ON CONFLICT (did, to_peer_did) WHERE kind = 'OFFER_ACCEPTED'
        DO UPDATE SET
            recorded_at = CURRENT_TIMESTAMP,
            delivered_at = NULL,
            contract_version = EXCLUDED.contract_version
        RETURNING id
    `
	var id string
	err := tx.GetContext(ctx, &id, statement,
		msg.DID, msg.FromPeerDID, msg.ToPeerDID, negotiationID, msg.ContractVersion)
	return id, err
}

func (r PostgresNegotiationOutboundRepo) enqueueWithdraw(ctx context.Context, tx *sqlx.Tx, msg db.NegotiationOutboundMessage) (string, error) {
	statement := `
        INSERT INTO contract_negotiation_outbound (
            did, from_peer_did, to_peer_did, kind, negotiation_id, decision_id, contract_version
        ) VALUES ($1, $2, $3, 'WITHDRAW'::contract_negotiation_message_kind, $4, NULL, $5)
        ON CONFLICT (negotiation_id, to_peer_did) WHERE kind = 'WITHDRAW'
        DO UPDATE SET
            recorded_at = CURRENT_TIMESTAMP,
            delivered_at = NULL
        RETURNING id
    `

	var id string
	err := tx.GetContext(ctx, &id, statement,
		msg.DID, msg.FromPeerDID, msg.ToPeerDID, msg.NegotiationID, msg.ContractVersion)

	return id, err
}

func (r PostgresNegotiationOutboundRepo) CancelUndeliveredProposal(ctx context.Context, tx *sqlx.Tx, negotiationID, toPeerDID string) error {
	statement := `
        DELETE FROM contract_negotiation_outbound
        WHERE negotiation_id = $1
          AND to_peer_did = $2
          AND kind = 'PROPOSAL'
          AND delivered_at IS NULL
    `
	_, err := tx.ExecContext(ctx, statement, negotiationID, toPeerDID)

	return err
}

func (r PostgresNegotiationOutboundRepo) enqueueProposal(ctx context.Context, tx *sqlx.Tx, msg db.NegotiationOutboundMessage) (string, error) {
	statement := `
        INSERT INTO contract_negotiation_outbound (
            did, from_peer_did, to_peer_did, kind, negotiation_id, decision_id, contract_version
        ) VALUES ($1, $2, $3, 'PROPOSAL'::contract_negotiation_message_kind, $4, NULL, $5)
        ON CONFLICT (negotiation_id, to_peer_did) WHERE kind = 'PROPOSAL'
        DO UPDATE SET
            recorded_at = CURRENT_TIMESTAMP,
            delivered_at = NULL
        RETURNING id
    `

	var id string
	err := tx.GetContext(ctx, &id, statement,
		msg.DID, msg.FromPeerDID, msg.ToPeerDID, msg.NegotiationID, msg.ContractVersion)
	return id, err
}

func (r PostgresNegotiationOutboundRepo) enqueueDecision(ctx context.Context, tx *sqlx.Tx, msg db.NegotiationOutboundMessage) (string, error) {
	if msg.DecisionID == nil || *msg.DecisionID == "" {
		return "", fmt.Errorf("decision outbound message requires decision_id")
	}

	statement := `
        INSERT INTO contract_negotiation_outbound (
            did, from_peer_did, to_peer_did, kind, negotiation_id, decision_id, contract_version
        ) VALUES ($1, $2, $3, $4::contract_negotiation_message_kind, $5, $6, $7)
        ON CONFLICT (decision_id, to_peer_did) WHERE kind IN ('ACCEPT', 'REJECT') AND decision_id IS NOT NULL
        DO UPDATE SET
            kind = EXCLUDED.kind,
            recorded_at = CURRENT_TIMESTAMP,
            delivered_at = NULL
        RETURNING id
    `
	var id string
	err := tx.GetContext(ctx, &id, statement,
		msg.DID, msg.FromPeerDID, msg.ToPeerDID, msg.Kind, msg.NegotiationID, *msg.DecisionID, msg.ContractVersion)

	return id, err
}

func (r PostgresNegotiationOutboundRepo) GetUndelivered(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]db.NegotiationOutboundMessage, error) {
	query := `
        SELECT id, did, from_peer_did, to_peer_did, kind::text, negotiation_id, decision_id,
               contract_version, recorded_at, delivered_at
        FROM contract_negotiation_outbound
        WHERE from_peer_did = $1 AND delivered_at IS NULL
        ORDER BY recorded_at, id
    `

	var rows []db.NegotiationOutboundMessage
	err := tx.SelectContext(ctx, &rows, query, fromPeerDID)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (r PostgresNegotiationOutboundRepo) GetUndeliveredForDID(ctx context.Context, tx *sqlx.Tx, fromPeerDID, did string) ([]db.NegotiationOutboundMessage, error) {
	query := `
        SELECT id, did, from_peer_did, to_peer_did, kind::text, negotiation_id, decision_id,
               contract_version, recorded_at, delivered_at
        FROM contract_negotiation_outbound
        WHERE from_peer_did = $1 AND did = $2 AND delivered_at IS NULL
        ORDER BY recorded_at, id
    `

	var rows []db.NegotiationOutboundMessage
	err := tx.SelectContext(ctx, &rows, query, fromPeerDID, did)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (r PostgresNegotiationOutboundRepo) MarkDelivered(ctx context.Context, tx *sqlx.Tx, id string) error {
	statement := `
        UPDATE contract_negotiation_outbound
        SET delivered_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `

	_, err := tx.ExecContext(ctx, statement, id)
	return err
}
