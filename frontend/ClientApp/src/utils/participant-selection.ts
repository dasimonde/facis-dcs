/**
 * The counterparty is the other DCS this contract is offered to and negotiated
 * with — a `did:web` peer (ADR-13). It is recorded as-is on the contract; there
 * is no JWT-`sub` binding/validation here or on the backend, so any
 * syntactically accepted `did:web` can be assigned (see the two-instance
 * peer-trust pack, features/17_peer_trust). Reviewer/approver/negotiator roles
 * are LOCAL RBAC roles held by this instance's own users, not part of contract
 * creation — each DCS runs its own workflow.
 */

export interface ParticipantSelection {
  /** Counterparty `did:web`, or empty for a purely local contract. */
  counterparty: string
}
