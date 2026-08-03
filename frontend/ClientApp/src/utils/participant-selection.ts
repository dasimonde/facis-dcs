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
  /**
   * The contractual role the creating organization takes in the contract's ODRL
   * rules (e.g. provider, customer). Binds the origin DID to that role's party
   * node; the counterpart role stays open until the counterparty signs.
   */
  originatorRole?: string
  /**
   * Organizations authorized to read this contract, by legal name, matched
   * against the OID4VP organization claim. Read authorization only.
   */
  parties?: string[]
}

/**
 * The contractual roles a template declares, read off its party placeholder
 * nodes (`…#party-<role>`), which is what the backend binds the originator to.
 */
export function declaredPartyRoles(document: { 'dcs:parties'?: unknown[] } | undefined): string[] {
  const nodes = document?.['dcs:parties'] ?? []
  const roles = new Set<string>()
  for (const node of nodes) {
    if (typeof node !== 'object' || node === null) continue
    const iri = (node as { '@id'?: unknown })['@id']
    if (typeof iri !== 'string') continue
    const marker = iri.lastIndexOf('#party-')
    if (marker === -1) continue
    const role = iri.slice(marker + '#party-'.length)
    if (role) roles.add(role)
  }
  return Array.from(roles)
}
