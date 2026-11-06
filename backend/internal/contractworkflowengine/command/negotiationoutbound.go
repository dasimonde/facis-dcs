package command

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// requireConfirmedNegotiationDelivery blocks when this instance still has
// undelivered outbound negotiation messages for the contract. An empty queue
// (including purely local contracts with no counterparty) is a no-op.
func requireConfirmedNegotiationDelivery(
	ctx context.Context,
	tx *sqlx.Tx,
	outbound db.NegotiationOutboundRepo,
	localPeer, did string,
) error {
	if outbound == nil {
		return nil
	}
	pending, err := outbound.GetUndeliveredForDID(ctx, tx, localPeer, did)
	if err != nil {
		return fmt.Errorf("could not check negotiation delivery: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}
	return ErrNegotiationSyncPending
}

// enqueueNegotiationOutbound records one undelivered outbound row toward every
// counterparty peer. Local domain writes must already have succeeded; peer
// downtime must not fail them — this queue is drained by the synchronizer.
func enqueueNegotiationOutbound(
	ctx context.Context,
	tx *sqlx.Tx,
	cRepo db.ContractRepo,
	outbound db.NegotiationOutboundRepo,
	localPeer, kind, negotiationID string,
	decisionID *string,
	did string,
	contractVersion int,
) error {
	if outbound == nil {
		return nil
	}

	contract, err := cRepo.ReadDataByDID(ctx, tx, did)

	if err != nil {
		return fmt.Errorf("could not read contract parties for negotiation outbound: %w", err)
	}

	if contract.Responsible == nil {
		return nil
	}

	for _, peer := range contract.Responsible.GetParties() {
		if identity.SameDIDWeb(peer, localPeer) {
			continue
		}

		_, err = outbound.Enqueue(ctx, tx, db.NegotiationOutboundMessage{
			DID:             did,
			FromPeerDID:     localPeer,
			ToPeerDID:       peer,
			Kind:            kind,
			NegotiationID:   negotiationID,
			DecisionID:      decisionID,
			ContractVersion: contractVersion,
		})

		if err != nil {
			return fmt.Errorf("could not enqueue %s negotiation outbound to %s: %w", kind, peer, err)
		}
	}

	return nil
}
