package dcstodcs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
)

// NegotiationSender delivers one outbound negotiation message to a peer.
type NegotiationSender interface {
	SendNegotiation(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSNegotiationMessageRequest) error
}

// HTTPNegotiationSender calls POST /peer/contracts/negotiation on the peer.
type HTTPNegotiationSender struct{}

func (HTTPNegotiationSender) SendNegotiation(ctx context.Context, peerDID string, req *dcstodcs.DCSToDCSNegotiationMessageRequest) error {
	client, err := peerSettlementClient(peerDID)
	if err != nil {
		return err
	}
	_, err = client.PostNegotiation(ctx, req)

	return err
}

// shipNegotiationMessagesForDID delivers undelivered outbound negotiation
// messages for one contract, in recorded order so a Proposal precedes its
// Accept/Reject when both are pending.
func (s *DCSToDCSSynchronizer) shipNegotiationMessagesForDID(ctx context.Context, did string) {
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		log.Printf(ctx, "could not read the own peer identity for negotiation ships: %v", err)
		return
	}

	if s.NegotiationSender == nil || s.NORepo == nil || s.NRepo == nil {
		return
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf(ctx, "could not start transaction: %v", err)
		return
	}

	pending, err := s.NORepo.GetUndeliveredForDID(ctx, tx, localPeer, did)
	_ = tx.Rollback()

	if err != nil {
		log.Printf(ctx, "could not read undelivered negotiation messages for %s: %v", did, err)
		return
	}

	for _, msg := range pending {
		err = s.deliverNegotiationMessage(ctx, msg)
		if err != nil {
			log.Printf(ctx, "negotiation ship for %s (%s) toward %s was not successful: %v",
				msg.DID, msg.Kind, msg.ToPeerDID, err)
		}
	}
}

func (s *DCSToDCSSynchronizer) retryUndeliveredNegotiationMessages(ctx context.Context) {
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		log.Printf(ctx, "could not read the own peer identity for negotiation retries: %v", err)
		return
	}

	if s.NegotiationSender == nil || s.NORepo == nil || s.NRepo == nil {
		return
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf(ctx, "could not start transaction: %v", err)
		return
	}

	pending, err := s.NORepo.GetUndelivered(ctx, tx, localPeer)
	_ = tx.Rollback()

	if err != nil {
		log.Printf(ctx, "could not read undelivered negotiation messages: %v", err)
		return
	}
	for _, msg := range pending {
		err = s.deliverNegotiationMessage(ctx, msg)
		if err != nil {
			log.Printf(ctx, "negotiation ship retry for %s (%s) toward %s was not successful: %v",
				msg.DID, msg.Kind, msg.ToPeerDID, err)
		}
	}
}

func (s *DCSToDCSSynchronizer) deliverNegotiationMessage(ctx context.Context, msg cwedb.NegotiationOutboundMessage) error {
	err := s.TrustGate.Check(ctx, msg.ToPeerDID, Outbound, msg.DID, "")
	if err != nil {
		return err
	}

	req, err := s.buildNegotiationShipRequest(ctx, msg)
	if err != nil {
		return err
	}

	body, err := NegotiationMessageBody(req)
	if err != nil {
		return err
	}
	req.SecretValue, req.SecretHash, err = SignPeerChallenge(&s.DIDDocument, OpPostNegotiation, req.FromPeerDid, msg.ToPeerDID, req.ContractIri, body)
	if err != nil {
		return err
	}

	err = s.NegotiationSender.SendNegotiation(ctx, msg.ToPeerDID, req)
	if err != nil {
		return err
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}

	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf(ctx, "could not rollback transaction: %v", err)
		}
	}(tx)

	err = s.NORepo.MarkDelivered(ctx, tx, msg.ID)
	if err != nil {
		return fmt.Errorf("could not mark negotiation message %s delivered: %w", msg.ID, err)
	}
	err = RecordPeerExchangeTx(ctx, tx, msg.DID, "post_negotiation", ExchangeSent, msg.ToPeerDID)
	if err != nil {
		return fmt.Errorf("could not audit negotiation message %s: %w", msg.ID, err)
	}

	return tx.Commit()
}

func (s *DCSToDCSSynchronizer) buildNegotiationShipRequest(ctx context.Context, msg cwedb.NegotiationOutboundMessage) (*dcstodcs.DCSToDCSNegotiationMessageRequest, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	req := &dcstodcs.DCSToDCSNegotiationMessageRequest{
		FromPeerDid:     msg.FromPeerDID,
		ContractIri:     msg.DID,
		MessageID:       msg.ID,
		Kind:            msg.Kind,
		NegotiationID:   msg.NegotiationID,
		DecisionID:      msg.DecisionID,
		ContractVersion: msg.ContractVersion,
	}

	switch msg.Kind {
	case cwedb.NegotiationMessageProposal:
		row, err := s.NRepo.ReadNegotiationByID(ctx, tx, msg.NegotiationID)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, fmt.Errorf("negotiation %s missing for outbound ship", msg.NegotiationID)
		}
		createdBy := row.CreatedBy
		req.CreatedBy = &createdBy
		createdAt := row.CreatedAt.UTC().Format(time.RFC3339)
		req.CreatedAt = &createdAt
		if row.ChangeRequest != nil {
			var decoded any
			err = json.Unmarshal(*row.ChangeRequest, &decoded)
			if err != nil {
				return nil, fmt.Errorf("decode change_request for negotiation %s: %w", msg.NegotiationID, err)
			}
			req.ChangeRequest = decoded
		}
	case cwedb.NegotiationMessageAccept, cwedb.NegotiationMessageReject:
		if msg.DecisionID == nil {
			return nil, fmt.Errorf("decision outbound message %s has no decision_id", msg.ID)
		}
		row, err := s.NRepo.ReadDecisionByID(ctx, tx, *msg.DecisionID)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, fmt.Errorf("decision %s missing for outbound ship", *msg.DecisionID)
		}
		negotiator := row.Negotiator
		req.Negotiator = &negotiator
		req.Decision = row.Decision
		req.RejectionReason = row.RejectionReason
	case cwedb.NegotiationMessageWithdraw:
		// Domain row is already deleted locally; the outbound row carries the
		// negotiation_id the peer must drop.
	case cwedb.NegotiationMessageOfferAccepted:
		// No domain negotiation row; the receiver only advances OFFERED →
		// NEGOTIATION on the origin copy.
	default:
		return nil, fmt.Errorf("unknown negotiation outbound kind %q", msg.Kind)
	}
	return req, nil
}

func isNegotiationDeliveryEvent(eventType string) bool {
	switch eventType {
	case cweeventtype.Negotiation.String(),
		cweeventtype.AcceptRespond.String(),
		cweeventtype.RejectRespond.String(),
		cweeventtype.WithdrawRespond.String(),
		cweeventtype.AcceptOffer.String():
		return true
	default:
		return false
	}
}
