package contractstate

import (
	"errors"
	"fmt"
)

// Event identifies a contract-workflow command/action that may attempt a
// state transition. Events are distinct from ContractState values: several
// events (e.g. EventSubmit) may be valid from more than one source state and
// may — depending on business logic that stays in the command handlers —
// land on more than one target state.
type Event string

const (
	// EventOffer: first transmission of a draft contract to the
	// counterparty (SRS 2.2.6). Triggers the PostSync broadcast.
	EventOffer Event = "OFFER"

	// EventWithdraw: initiator retracts the contract before it is approved.
	EventWithdraw Event = "WITHDRAW"

	// EventSubmit: the overloaded submit command (see command/submit.go).
	// Its concrete effect (which of the allowed target states is actually
	// reached) is decided by the existing imperative task-orchestration
	// logic; the transition table only bounds which outcomes are legal.
	EventSubmit Event = "SUBMIT"

	// EventNegotiate/EventAcceptNegotiation/EventRejectNegotiation: manage
	// individual negotiation change-request records. None of these change
	// the contract's own state (they stay within NEGOTIATION), but they are
	// still validated against the table per the C4 requirement that every
	// command handler checks against it.
	EventNegotiate         Event = "NEGOTIATE"
	EventAcceptNegotiation Event = "ACCEPT_NEGOTIATION"
	EventRejectNegotiation Event = "REJECT_NEGOTIATION"

	// EventApprove/EventReject: the final approval decision.
	EventApprove Event = "APPROVE"
	EventReject  Event = "REJECT"

	// EventTerminate: reachable from any non-terminal state.
	EventTerminate Event = "TERMINATE"

	// EventSign: Signature Management applies a signature.
	EventSign Event = "SIGN"

	// EventDeploy: drives the SIGNED -> ACTIVE transition. The deploy command
	// (command/deploy.go) validates this edge before dispatching to the
	// Contract Target System, and the target's ack callback (command/callback.go)
	// validates it again before flipping the contract to ACTIVE. From ACTIVE
	// the same event is an idempotent re-dispatch (ACTIVE -> ACTIVE): signing
	// completion auto-deploys and the real target acknowledges within moments
	// (DCS-FR-CWE-06/SM-12), so a manual /contract/deploy — and the second
	// ack it produces — must stay valid for an already-activated contract.
	EventDeploy Event = "DEPLOY"

	// EventRevoke: signingmanagement/command/revoke.go transitions the contract
	// Signed/Active -> Revoked after a signature is revoked (DCS-OR-C2PA-006,
	// C2PA lifecycle banner "suspended").
	EventRevoke Event = "REVOKE"

	// EventUpdate: editing draft contract data before submission.
	EventUpdate Event = "UPDATE"
)

// ErrInvalidTransition is wrapped by every error ValidateTransition (and the
// command handlers built on top of it) return. Service-layer code checks
// errors.Is(err, ErrInvalidTransition) to classify the failure as a client
// error (HTTP 400) rather than an internal error (HTTP 500).
var ErrInvalidTransition = errors.New("invalid contract state transition")

// Transitions is the single source of truth for which (from-state, event)
// pairs are allowed and which target state(s) each may produce. Command
// handlers validate against this table instead of re-implementing ad hoc
// state checks; the existing task-orchestration logic (negotiation rounds,
// review/approval reopening, etc.) stays as-is and simply picks which of the
// table's allowed outcomes actually applies for a given call.
var Transitions = map[ContractState]map[Event][]ContractState{
	Draft: {
		EventOffer:     {Offered},
		EventSubmit:    {Negotiation},
		EventUpdate:    {Draft},
		EventTerminate: {Terminated},
	},
	Rejected: {
		EventSubmit:    {Negotiation},
		EventTerminate: {Terminated},
	},
	Offered: {
		// Offered -> Negotiation: the creator submits the offered contract to
		// start the negotiation round (mirrors Draft -> Negotiation; see
		// command/submit.go's Offered branch). Without this edge the
		// DRAFT -> OFFERED -> NEGOTIATION -> SUBMITTED -> ... sequence is
		// unreachable once a contract has been offered.
		EventSubmit: {Negotiation},
		// Offered -> Negotiation via EventNegotiate: the COUNTERPARTY (Responder)
		// opens the negotiation by proposing a redline on its received offer (SRS §4
		// Contract Negotiation & Review — the Responder may accept, negotiate, or
		// refuse; DCS-IR-CWE-03 exchange responses/redlines; DCS-FR-CWE-18). Its
		// authority comes from being the designated counterparty, not from local
		// negotiator RBAC (see command/negotiate.go's inbound branch) — the
		// Creator-only EventSubmit is not the counterparty's path.
		EventNegotiate: {Negotiation},
		EventWithdraw:  {Withdrawn},
		EventTerminate: {Terminated},
	},
	Negotiation: {
		// Negotiation -> Negotiation covers both "still waiting on other
		// negotiators" and "merged an accepted change request, new round
		// starts" (submit.go); Negotiation -> Submitted is reached once all
		// negotiators have accepted and there is nothing left to merge.
		EventSubmit:            {Negotiation, Submitted},
		EventWithdraw:          {Withdrawn},
		EventNegotiate:         {Negotiation},
		EventAcceptNegotiation: {Negotiation},
		EventRejectNegotiation: {Negotiation},
		EventTerminate:         {Terminated},
	},
	Submitted: {
		// Submitted -> Reviewed (all reviewers approved), Submitted ->
		// Negotiation (a reviewer rejected), Submitted -> Submitted (some
		// reviewers still pending).
		EventSubmit:    {Reviewed, Negotiation, Submitted},
		EventWithdraw:  {Withdrawn},
		EventTerminate: {Terminated},
	},
	Reviewed: {
		// Submitted from Reviewed represents an approver sending the
		// contract back for further review (reopens review+approval tasks).
		EventSubmit:    {Submitted},
		EventApprove:   {Approved},
		EventReject:    {Rejected},
		EventWithdraw:  {Withdrawn},
		EventTerminate: {Terminated},
	},
	Approved: {
		EventSign:      {Signed},
		EventTerminate: {Terminated},
		// NOTE: Withdraw is intentionally NOT allowed once Approved.
	},
	Signed: {
		// Signed -> Signed: a further signatory applies their signature on a
		// multi-signer contract (DCS-FR-SM-07/-17). The deploy command gates
		// on ALL declared signature fields being signed, so a partially
		// signed contract can never activate.
		EventSign:      {Signed},
		EventDeploy:    {Active},
		EventRevoke:    {Revoked},
		EventTerminate: {Terminated},
	},
	Active: {
		EventDeploy:    {Active},
		EventRevoke:    {Revoked},
		EventTerminate: {Terminated},
	},
	Revoked: {
		// Re-signing path (SRS UC-15): a revoked contract can return to
		// Approved to allow re-signing. REVOKED itself is reachable via
		// signingmanagement/command/revoke.go (EventRevoke from Signed/Active).
		EventApprove:   {Approved},
		EventTerminate: {Terminated},
	},
	Withdrawn: {
		EventTerminate: {Terminated},
	},
	Expired: {
		EventTerminate: {Terminated},
	},
}

// EventAllowed reports whether evt has any declared outcome from state from.
func EventAllowed(from ContractState, evt Event) bool {
	_, ok := Transitions[from][evt]
	return ok
}

// IsAllowed reports whether transitioning from "from" to "to" via evt is a
// declared outcome in the table. A no-op transition (from == to) is always
// allowed regardless of the table, since it does not change any state.
func IsAllowed(from ContractState, evt Event, to ContractState) bool {
	if from == to {
		return true
	}
	outcomes, ok := Transitions[from][evt]
	if !ok {
		return false
	}
	for _, outcome := range outcomes {
		if outcome == to {
			return true
		}
	}
	return false
}

// ValidateTransition returns an ErrInvalidTransition-wrapped error if evt is
// not a declared event from state "from", else nil. Command handlers call
// this before applying an event's business logic.
func ValidateTransition(from ContractState, evt Event) error {
	if !EventAllowed(from, evt) {
		return fmt.Errorf("%w: event %s is not allowed from state %s", ErrInvalidTransition, evt, from)
	}
	return nil
}

// ValidateOutcome returns an ErrInvalidTransition-wrapped error if moving
// from "from" to "to" via evt is not a declared outcome, else nil. This is a
// safety net command handlers can call after computing their intended next
// state, ensuring the table remains the single source of truth for exactly
// which outcomes are legal (not just which events are legal).
func ValidateOutcome(from ContractState, evt Event, to ContractState) error {
	if !IsAllowed(from, evt, to) {
		return fmt.Errorf("%w: event %s cannot move state %s to %s", ErrInvalidTransition, evt, from, to)
	}
	return nil
}
