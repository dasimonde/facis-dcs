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

export function contractPartyRoleFromReference(iri: string): string {
  const marker = iri.lastIndexOf('#party-')
  if (marker === -1) return ''
  try {
    return decodeURIComponent(iri.slice(marker + '#party-'.length))
  } catch {
    return ''
  }
}

/**
 * The contractual roles a template declares in its top-level ODRL rules.
 * Rules may reverse assigner/assignee direction; the role set is their
 * deduplicated union.
 */
export function declaredPartyRoles(document: { 'dcs:policies'?: unknown } | undefined): string[] {
  const policies = document?.['dcs:policies']
  const rules = Array.isArray(policies)
    ? policies
    : policies && typeof policies === 'object'
      ? ['odrl:permission', 'odrl:prohibition', 'odrl:obligation'].flatMap((bucket) => {
          const value = (policies as Record<string, unknown>)[bucket]
          return Array.isArray(value) ? value : []
        })
      : []
  const roles = new Set<string>()
  for (const rule of rules) {
    if (typeof rule !== 'object' || rule === null) continue
    for (const side of ['odrl:assigner', 'odrl:assignee']) {
      const reference = (rule as Record<string, unknown>)[side]
      if (typeof reference !== 'object' || reference === null) continue
      const iri = (reference as { '@id'?: unknown })['@id']
      if (typeof iri !== 'string') continue
      const role = contractPartyRoleFromReference(iri)
      if (role) roles.add(role)
    }
  }
  return Array.from(roles)
}
