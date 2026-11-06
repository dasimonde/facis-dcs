import { poaRolesDisplay, signerDisplayName } from '@/utils/credential-chain-evidence'
import { findingsVerdict, signatureLevelLabel, signatureStatusIndicator } from '@/utils/signature-verdict'
import type { SignatureViewItem } from '@/services/signature-management-service'

const LEVEL_RANK: Record<string, number> = { SES: 0, AES: 1, QES: 2 }

export interface PartyComplianceSection {
  /** Display title: organization DID when known, else signer DID. */
  party_label: string
  organization: string | null
  /** Human-readable signer (cert subject preferred), e.g. "DCS Signatory MAX". */
  signer: string
  signer_did: string
  field_name?: string
  /** Signature lifecycle status badge (SIGNED → ACTIVE/SIGNED) — not a compliance verdict. */
  status_label: string
  status_cls: string
  credential_type: string
  required_credential_type: string
  level_meets_requirement: boolean
  /** PoA roles when present; null when not published on local evidence. */
  authorized_role: string | null
  signed_at?: string
  /** Evidence-based assessment text; never "Compliant" from SIGNED alone. */
  assessment: string
  /** Findings that mention this signer DID (or empty). */
  non_conformance: string[]
  non_conformance_summary: string
}

export interface PartyComplianceReport {
  scope_note: string
  parties: PartyComplianceSection[]
  /** Findings that could not be tied to a local signer DID. */
  contract_findings: string[]
}

/** True when a free-text finding names this signature's signer DID. */
export function findingMentionsSigner(finding: string, signerDid: string): boolean {
  const did = signerDid.trim()
  if (!did) return false
  return finding.toLowerCase().includes(did.toLowerCase())
}

export function partitionFindingsBySigner(
  findings: string[],
  signatures: SignatureViewItem[],
): { bySigner: Map<string, string[]>; unassigned: string[] } {
  const bySigner = new Map<string, string[]>()
  for (const sig of signatures) {
    bySigner.set(sig.signer_did, [])
  }
  const unassigned: string[] = []
  for (const finding of findings) {
    const matched = signatures.filter((sig) => findingMentionsSigner(finding, sig.signer_did))
    if (matched.length === 1) {
      bySigner.get(matched[0]!.signer_did)!.push(finding)
      continue
    }
    // Ambiguous (multiple DIDs) or none → keep at contract level.
    unassigned.push(finding)
  }
  return { bySigner, unassigned }
}

function requiredLevel(sig: SignatureViewItem): string {
  return (sig.required_credential_type ?? 'AES').toUpperCase()
}

function levelMeetsRequirement(sig: SignatureViewItem): boolean {
  const achieved = signatureLevelLabel(sig.credential_type)
  if (achieved === 'NOT ESTABLISHED') return false
  const achievedRank = LEVEL_RANK[achieved] ?? 0
  const requiredRank = LEVEL_RANK[requiredLevel(sig)] ?? 1
  return achievedRank >= requiredRank
}

/**
 * Party-level assessment from signature evidence + attributable findings.
 * SIGNED alone never yields a "compliant" claim.
 */
export function partyAssessment(sig: SignatureViewItem, partyFindings: string[]): string {
  const status = sig.status.trim().toUpperCase()
  if (status === 'REVOKED') {
    return 'Signature revoked — not compliant for this party'
  }
  if (status && status !== 'SIGNED') {
    return `Signature not finalized (status=${status})`
  }

  const level = signatureLevelLabel(sig.credential_type)
  if (level === 'NOT ESTABLISHED') {
    return 'Signature level not established'
  }

  const parts: string[] = []
  if (levelMeetsRequirement(sig)) {
    parts.push(`Signature level meets requirement (${level} ≥ ${requiredLevel(sig)})`)
  } else {
    parts.push(`Signature level below requirement (achieved ${level}, required ${requiredLevel(sig)})`)
  }

  if (partyFindings.length) {
    const verdict = findingsVerdict(partyFindings)
    if (verdict === 'fail') {
      parts.push('Party-specific compliance findings report defects')
    } else if (verdict === 'indeterminate') {
      parts.push('Party-specific compliance findings are inconclusive')
    }
  } else {
    parts.push('No party-specific findings attributable to this signer')
  }

  return parts.join('. ')
}

export function buildPartyComplianceReport(
  signatures: SignatureViewItem[],
  complianceFindings: string[] = [],
  integrityFindings: string[] = [],
): PartyComplianceReport {
  const allFindings = [...complianceFindings, ...integrityFindings]
  const { bySigner, unassigned } = partitionFindingsBySigner(allFindings, signatures)

  const parties: PartyComplianceSection[] = signatures.map((sig) => {
    const partyFindings = bySigner.get(sig.signer_did) ?? []
    const orgRaw = sig.poa_organization?.trim()
    let org: string | null = null
    if (orgRaw) org = orgRaw
    const status = signatureStatusIndicator(sig.status, 'Signed')
    return {
      party_label: org ?? sig.signer_did,
      organization: org,
      signer: signerDisplayName(sig),
      signer_did: sig.signer_did,
      field_name: sig.field_name,
      status_label: status.label,
      status_cls: status.cls,
      credential_type: signatureLevelLabel(sig.credential_type),
      required_credential_type: requiredLevel(sig),
      level_meets_requirement: levelMeetsRequirement(sig),
      authorized_role: poaRolesDisplay(sig),
      signed_at: sig.signed_at,
      assessment: partyAssessment(sig, partyFindings),
      non_conformance: partyFindings,
      non_conformance_summary: partyFindings.length ? partyFindings.join('; ') : 'No party-specific findings',
    }
  })

  return {
    scope_note: 'This report includes signing evidence available on the current DCS instance.',
    parties,
    contract_findings: unassigned,
  }
}
