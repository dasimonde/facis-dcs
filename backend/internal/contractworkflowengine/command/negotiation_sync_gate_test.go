package command

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type outboundRepoFake struct {
	pending []db.NegotiationOutboundMessage
	err     error
}

func (r *outboundRepoFake) Enqueue(context.Context, *sqlx.Tx, db.NegotiationOutboundMessage) (string, error) {
	return "", nil
}
func (r *outboundRepoFake) CancelUndeliveredProposal(context.Context, *sqlx.Tx, string, string) error {
	return nil
}
func (r *outboundRepoFake) GetUndelivered(context.Context, *sqlx.Tx, string) ([]db.NegotiationOutboundMessage, error) {
	return nil, nil
}
func (r *outboundRepoFake) GetUndeliveredForDID(context.Context, *sqlx.Tx, string, string) ([]db.NegotiationOutboundMessage, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.pending, nil
}
func (r *outboundRepoFake) MarkDelivered(context.Context, *sqlx.Tx, string) error { return nil }

func TestRequireConfirmedNegotiationDeliveryAllowsEmptyQueue(t *testing.T) {
	err := requireConfirmedNegotiationDelivery(context.Background(), nil, &outboundRepoFake{}, "did:web:a", "contract")
	if err != nil {
		t.Fatalf("empty queue must allow Accept/Submit: %v", err)
	}
}

func TestRequireConfirmedNegotiationDeliveryBlocksWhenPending(t *testing.T) {
	err := requireConfirmedNegotiationDelivery(context.Background(), nil, &outboundRepoFake{
		pending: []db.NegotiationOutboundMessage{{
			ID: "1", DID: "contract", Kind: db.NegotiationMessageProposal, RecordedAt: time.Now(),
		}},
	}, "did:web:a", "contract")
	if err != ErrNegotiationSyncPending {
		t.Fatalf("want ErrNegotiationSyncPending, got %v", err)
	}
}

// Peer downtime / lost ACKs leave undelivered outbound rows; Accept and Submit
// must stay blocked until every prior Proposal/Decision is confirmed.
func TestRequireConfirmedNegotiationDeliveryBlocksOnLostAcksAndDowntime(t *testing.T) {
	err := requireConfirmedNegotiationDelivery(context.Background(), nil, &outboundRepoFake{
		pending: []db.NegotiationOutboundMessage{
			{ID: "1", DID: "contract", Kind: db.NegotiationMessageProposal, RecordedAt: time.Now().Add(-time.Minute)},
			{ID: "2", DID: "contract", Kind: db.NegotiationMessageAccept, RecordedAt: time.Now()},
		},
	}, "did:web:a", "contract")
	if err != ErrNegotiationSyncPending {
		t.Fatalf("want ErrNegotiationSyncPending for undelivered Accept after Proposal, got %v", err)
	}
}

func TestRequireConfirmedNegotiationDeliveryBlocksOnDuplicatePendingRows(t *testing.T) {
	// Out-of-order or retried delivery can leave more than one undelivered row
	// for the same decision; the gate is presence of any pending ACK, not
	// uniqueness of the queue.
	now := time.Now()
	err := requireConfirmedNegotiationDelivery(context.Background(), nil, &outboundRepoFake{
		pending: []db.NegotiationOutboundMessage{
			{ID: "a", DID: "contract", Kind: db.NegotiationMessageProposal, RecordedAt: now},
			{ID: "a-retry", DID: "contract", Kind: db.NegotiationMessageProposal, RecordedAt: now.Add(time.Second)},
		},
	}, "did:web:a", "contract")
	if err != ErrNegotiationSyncPending {
		t.Fatalf("want ErrNegotiationSyncPending for duplicate pending deliveries, got %v", err)
	}
}
