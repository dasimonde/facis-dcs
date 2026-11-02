// The signature facts of one signature-management audit entry (DCS-FR-SM-19):
// the signer, the credential used and the outcome, read from its event_data.
export interface SignatureAuditFacts {
  signer: string
  credential: string
  outcome: string
  error: string
}

export function auditFacts(eventData: unknown): SignatureAuditFacts {
  const data = (eventData && typeof eventData === 'object' ? eventData : {}) as Record<string, unknown>
  const text = (value: unknown) => (typeof value === 'string' ? value : '')
  const signers = Array.isArray(data.signer_dids) ? data.signer_dids.filter((d) => typeof d === 'string') : []
  return {
    signer: text(data.signer_did) || signers.join(', '),
    credential: text(data.credential_type),
    outcome: text(data.outcome),
    error: text(data.error),
  }
}
