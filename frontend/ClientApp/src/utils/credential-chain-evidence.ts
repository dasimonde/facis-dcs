import type { SignatureViewItem } from '@/services/signature-management-service'

/** Local signing-evidence fields for the SM-26 Signing Credentials panel. */
export type CredentialChainEvidence = Pick<
  SignatureViewItem,
  'signer_did' | 'field_name' | 'signer_cert_subject' | 'poa_organization' | 'poa_roles' | 'summary_issuer'
>

/** Human-readable signer label (cert CN preferred). */
export function signerDisplayName(sig: CredentialChainEvidence): string {
  const subject = sig.signer_cert_subject?.trim()
  if (subject) return subject
  return sig.signer_did
}

export function summaryIssuerDisplay(sig: CredentialChainEvidence): string | null {
  const issuer = sig.summary_issuer?.trim()
  if (!issuer) return null
  return issuer
}

export function poaOrganizationDisplay(sig: CredentialChainEvidence): string | null {
  const org = sig.poa_organization?.trim()
  if (!org) return null
  return org
}

export function poaRolesDisplay(sig: CredentialChainEvidence): string | null {
  const roles = (sig.poa_roles ?? []).map((r) => r.trim()).filter(Boolean)
  if (!roles.length) return null
  return roles.join(', ')
}
