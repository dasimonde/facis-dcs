// Verdict classification for the signature-verification data the signing and
// compliance dashboards both render: the flat finding strings returned by
// GET /signature/view, POST /signature/validate and POST /signature/compliance,
// the EU DSS indication, and a signature's credential status and level.
//
// The keyword sets live here once. Held per view they drifted, and a dashboard
// that classifies a finding the other one flags reports a verdict the backend
// never gave.

export type FindingVerdict = 'pass' | 'indeterminate' | 'fail'

export interface VerdictIndicator {
  label: string
  cls: string
}

// Findings that name a proven defect: something was checked and came back wrong.
const FAILURE_KEYWORDS =
  /(mismatch|drift detected|does not match|failed|could not|missing|no longer|revoked|power of attorney)/i

// Findings the backend could not determine — an unreachable status service, a
// DSS that declined to conclude, a credential level that was never established.
// The backend stopped failing open and now says so ("UNKNOWN (status service
// unreachable)"); rendering that as a pass reinstates the failure-open verdict
// in the UI, and rendering it as a failure asserts a defect nobody observed.
// Only findings the failure set does not already claim are examined here.
const INDETERMINATE_KEYWORDS = /(unknown|unreachable|not established|not determined|not available)/i

// A finding that names its own verdict is taken at its word. The backend labels a
// withheld verdict "indeterminate" explicitly and appends the reason it could not
// conclude, and that reason routinely contains a failure keyword ("could not
// resolve", "fetching did.json failed") — inferring 'fail' from the explanation
// asserts a defect nobody observed.
const DECLARED_INDETERMINATE = /\bindeterminate\b/i

export function findingVerdict(finding: string): FindingVerdict {
  if (DECLARED_INDETERMINATE.test(finding)) return 'indeterminate'
  if (FAILURE_KEYWORDS.test(finding)) return 'fail'
  return INDETERMINATE_KEYWORDS.test(finding) ? 'indeterminate' : 'pass'
}

const findingIndicators: Record<FindingVerdict, VerdictIndicator> = {
  pass: { label: 'PASS', cls: 'badge-success' },
  indeterminate: { label: 'INDETERMINATE', cls: 'badge-warning' },
  fail: { label: 'FAIL', cls: 'badge-error' },
}

export function findingIndicator(finding: string): VerdictIndicator {
  return findingIndicators[findingVerdict(finding)]
}

// The verdict over a whole finding list is its worst member: one undetermined
// check is enough to withhold a clean verdict from the set.
export function findingsVerdict(findings: string[]): FindingVerdict {
  let verdict: FindingVerdict = 'pass'
  for (const finding of findings) {
    const current = findingVerdict(finding)
    if (current === 'fail') return 'fail'
    if (current === 'indeterminate') verdict = 'indeterminate'
  }
  return verdict
}

export function dssIndicator(indication: string | undefined): VerdictIndicator {
  switch ((indication ?? '').toUpperCase()) {
    case 'TOTAL-PASSED':
      return { label: 'PASSED', cls: 'badge-success' }
    case 'INDETERMINATE':
      return { label: 'INDETERMINATE', cls: 'badge-warning' }
    case 'TOTAL-FAILED':
      return { label: 'FAILED', cls: 'badge-error' }
    default:
      return { label: indication ?? 'Unknown', cls: 'badge-ghost' }
  }
}

// A signature record's status (signingstatus.SigningStatus). Only the statuses
// the backend defines get a verdict badge; anything else is shown as it arrived
// rather than defaulting to a green "not revoked, therefore in force".
// activeLabel names the in-force state in the caller's vocabulary — the signer
// dashboard says SIGNED, the compliance viewer reads the same record as the
// credential's ACTIVE status.
export function signatureStatusIndicator(status: string, activeLabel = 'ACTIVE'): VerdictIndicator {
  switch (status.trim().toUpperCase()) {
    case 'SIGNED':
      return { label: activeLabel, cls: 'badge-success' }
    case 'PENDING':
      return { label: 'PENDING', cls: 'badge-warning' }
    case 'REVOKED':
      return { label: 'REVOKED', cls: 'badge-error' }
    default:
      return { label: status.trim().toUpperCase() || 'UNKNOWN', cls: 'badge-ghost' }
  }
}

// DCS-FR-SM-21: the signature level (SES/AES/QES) the signature actually
// ACHIEVED, recorded from what DSS validated at submit (ADR-20) and never
// re-derived here. An absent level means the system does not know it, which is
// not the same as it being AES: the signing path deliberately leaves the claim
// out rather than seal a guess into the issuer-signed credential, so a default
// here would re-assert exactly the attestation the backend refused to make.
export function signatureLevelLabel(credentialType: string | undefined): string {
  const level = credentialType?.trim()
  return level ? level.toUpperCase() : 'NOT ESTABLISHED'
}

export function signatureLevelBadgeClass(level: string): string {
  switch (level) {
    case 'QES':
      return 'badge-secondary'
    case 'AES':
      return 'badge-info'
    default:
      return 'badge-ghost'
  }
}
