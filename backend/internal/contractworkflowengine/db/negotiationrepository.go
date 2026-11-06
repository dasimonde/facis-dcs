package db

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

// ErrNoMatchingDecision indicates a respond (accept/reject) call, or a
// created_by lookup for the conflict-of-interest check, matched no row —
// e.g. an unknown negotiation id, an already-decided decision, or (the
// pre-existing rows-affected check in PostgresNegotiationRepo.Accept/Reject)
// a decision row that does not belong to the calling negotiator.
var ErrNoMatchingDecision = errors.New("no matching negotiation decision for this party")

type NegotiationCreateData struct {
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	ChangeRequest   *datatype.JSON `db:"change_request"`
	CreatedBy       string         `db:"created_by"`
}

type NegotiationData struct {
	ID              string         `db:"id"`
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	ChangeRequest   *datatype.JSON `db:"change_request"`
	Negotiator      string         `db:"negotiator"`
	Decision        *string        `db:"decision"`
	RejectionReason *string        `db:"rejection_reason"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	// SupersededBy is the JSON array written by MarkSuperseded — nil until the
	// round folds, and on a request whose content survived it. It is the only
	// thing distinguishing an accepted request that shaped the contract from
	// one that was accepted and then discarded, since Decision stays ACCEPTED
	// either way.
	SupersededBy *datatype.JSON `db:"superseded_by"`
}

// NegotiationSupersession records that folding a negotiation round dropped
// part of one accepted change request in favour of a later one. The fold is
// last-accepted-wins per field (see negotiationmerging.MergeChangeRequests),
// so an accepted request can leave nothing behind while its decision still
// reads ACCEPTED; Fields names the change-request keys whose values did not
// reach the merged version.
type NegotiationSupersession struct {
	NegotiationID  string   `json:"negotiation_id"`
	SupersededByID string   `json:"superseded_by"`
	Fields         []string `json:"fields"`
}

// NegotiationChangeData is one accepted change request of a round. CreatedAt
// carries the row's proposal time because the merge folds the requests in that
// order (see MergeChangeRequests): later accepted requests overwrite earlier
// ones field by field, so the fold is only well-defined against a total order.
type NegotiationChangeData struct {
	ID            string         `db:"id"`
	ChangeRequest *datatype.JSON `db:"change_request"`
	CreatedAt     time.Time      `db:"created_at"`
}

type NegotiationDecisionData struct {
	ID              string  `db:"id"`
	NegotiationID   string  `db:"negotiation_id"`
	Negotiator      string  `db:"negotiator"`
	Decision        *string `db:"decision"`
	RejectionReason *string `db:"rejection_reason"`
}

// NegotiationDraftData is a party-private staged change request (SRS §3.1.1
// Contract Negotiation UI "Save draft"): one row per (contract, author),
// never replicated to the peer.
type NegotiationDraftData struct {
	ContractDID   string         `db:"contract_did"`
	SavedBy       string         `db:"saved_by"`
	ChangeRequest *datatype.JSON `db:"change_request"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type NegotiationRepo interface {
	// Create inserts the proposal and open decision slots. It returns the new
	// negotiation id (stable wire identity for peer delivery) and created_at.
	Create(ctx context.Context, tx *sqlx.Tx, data NegotiationCreateData, negotiators []string) (id string, createdAt *time.Time, err error)
	// ReadDecisionID returns the decision row for (negotiation, negotiator),
	// used when enqueueing an Accept/Reject outbound message.
	ReadDecisionID(ctx context.Context, tx *sqlx.Tx, negotiationID, negotiator string) (string, error)
	// ReadNegotiationByID returns the proposal row used to build an outbound
	// ship payload. Nil when the id is unknown.
	ReadNegotiationByID(ctx context.Context, tx *sqlx.Tx, id string) (*NegotiationData, error)
	// ReadDecisionByID returns one decision row by its primary key.
	ReadDecisionByID(ctx context.Context, tx *sqlx.Tx, id string) (*NegotiationDecisionData, error)
	UpsertDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string, changeRequest *datatype.JSON) error
	// ReadDraft returns nil when the author has no stored draft for the contract.
	ReadDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string) (*NegotiationDraftData, error)
	DeleteDraft(ctx context.Context, tx *sqlx.Tx, contractDID string, savedBy string) error
	Accept(ctx context.Context, tx *sqlx.Tx, id string, acceptedBy string) error
	Reject(ctx context.Context, tx *sqlx.Tx, id string, rejectedBy string, rejectionReason *string) error
	// DeleteByID removes a proposal and its decisions (CASCADE). Used when the
	// author withdraws before any counterparty has accepted.
	DeleteByID(ctx context.Context, tx *sqlx.Tx, id string) error
	// HasAcceptedDecision reports whether any decision on the proposal is
	// ACCEPTED — the local withdraw gate (peer Accept already durable here).
	HasAcceptedDecision(ctx context.Context, tx *sqlx.Tx, negotiationID string) (bool, error)
	// RecordWithdrawn writes a tombstone so late Accept/Reject/Proposal ships
	// after a partition can ACK as no-ops instead of retrying forever.
	RecordWithdrawn(ctx context.Context, tx *sqlx.Tx, negotiationID, did string) error
	// IsWithdrawn reports whether RecordWithdrawn has been called for this id.
	IsWithdrawn(ctx context.Context, tx *sqlx.Tx, negotiationID string) (bool, error)
	// EnsureOpenDecision inserts an undecided decision for (negotiation,
	// negotiator) when none exists. Idempotent for Proposal re-delivery.
	EnsureOpenDecision(ctx context.Context, tx *sqlx.Tx, negotiationID, negotiator string) error
	ReadAllByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationData, error)
	// ReadAllAcceptedByContractDIDAndVersion returns the round's accepted
	// change requests ordered by (created_at, id). The order is part of the
	// contract: the merge that consumes them is order-dependent.
	ReadAllAcceptedByContractDIDAndVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) ([]NegotiationChangeData, error)
	// HasOpenNegotiationDecisions is true when any proposal for the version still
	// needs handling before Submit (local Accept/Reject owed, or caller's own
	// proposal waiting on a counterparty). Author Not-required nulls are ignored.
	HasOpenNegotiationDecisions(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int, negotiator string, caller string) (bool, error)
	HasNegotiationForContractVersion(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) (bool, error)
	ReadAllNegotiationDecisionsByContractDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationDecisionData, error)
	ReadCreatedByByNegotiationID(ctx context.Context, tx *sqlx.Tx, id string) (string, error)
	// MarkSuperseded annotates the change requests whose content the fold of a
	// round discarded, each naming the later accepted request that beat it.
	// Written in the same transaction as the merged contract version, so the
	// annotation and the document it explains land together.
	MarkSuperseded(ctx context.Context, tx *sqlx.Tx, supersessions []NegotiationSupersession) error

	RemoteCreateOrUpdateNegotiation(ctx context.Context, tx *sqlx.Tx, data NegotiationData) error
	RemoteCreateOrUpdateNegotiationDecision(ctx context.Context, tx *sqlx.Tx, data NegotiationDecisionData) error
}
