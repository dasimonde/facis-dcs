import { type DcsContractData, isDcsClause } from '@/models/dcs-jsonld'

/** Content-policy finding candidate for clause attribution (from PAC timeline event_data). */
export interface ClauseFindingInput {
  rule_id?: string
  severity?: string
  message: string
  path?: string
  field_iri?: string
  result?: string
}

export interface ClauseComplianceSection {
  clause_id: string
  title: string
  findings: ClauseFindingInput[]
  non_conformance_summary: string
  assessment: string
  status?: string
  assessment_run?: string
  applicable_checks?: string
  verdict_note?: string
}

export interface ClauseComplianceReport {
  scope_note: string
  clauses: ClauseComplianceSection[]
  /** Findings that could not be tied to exactly one clause @id. */
  unassigned_findings: ClauseFindingInput[]
  clauses_available: boolean
  findings_available: boolean
  clauses_unavailable_reason: string | null
  findings_unavailable_reason: string | null
}

/** One applicable-check chip: human label, optional taxonomy/shape @id for hover. */
export interface ApplicableCheckChip {
  label: string
  id?: string
}

/**
 * Parse the clause-report `applicable_checks` display string into label chips.
 * Domain-field checks are encoded as `Label (https://…#field-…)` by the backend;
 * bare tokens (PaymentClauseShape, None, Not established) have no id.
 */
export function parseApplicableChecks(raw: string | undefined | null): ApplicableCheckChip[] {
  const s = (raw ?? '').trim()
  if (!s) return [{ label: 'Not established' }]
  if (s === 'None' || s === 'Not established') return [{ label: s }]

  const parts = splitApplicableCheckParts(s)
  const out: ApplicableCheckChip[] = []
  for (const part of parts) {
    const m = /^(.*) \((.+)\)$/.exec(part)
    if (m) {
      const label = m[1]!.trim()
      const id = m[2]!.trim()
      if (label) out.push({ label, id: id || undefined })
      continue
    }
    if (part) out.push({ label: part })
  }
  return out.length ? out : [{ label: s }]
}

/** Split on commas that are not inside parentheses. */
function splitApplicableCheckParts(s: string): string[] {
  const out: string[] = []
  let depth = 0
  let cur = ''
  for (let i = 0; i < s.length; i++) {
    const c = s[i]!
    if (c === '(') depth++
    if (c === ')') depth = Math.max(0, depth - 1)
    if (c === ',' && depth === 0) {
      if (cur.trim()) out.push(cur.trim())
      cur = ''
      if (s[i + 1] === ' ') i++
      continue
    }
    cur += c
  }
  if (cur.trim()) out.push(cur.trim())
  return out
}

export function extractClausesFromContractData(data: unknown): { id: string; title: string }[] {
  if (!data || typeof data !== 'object') return []
  const cd = data as DcsContractData
  const blocks = cd['dcs:documentStructure']?.['dcs:blocks']?.['@list']
  if (!Array.isArray(blocks)) return []
  const out: { id: string; title: string }[] = []
  for (const block of blocks) {
    if (!isDcsClause(block)) continue
    const id = typeof block['@id'] === 'string' ? block['@id'].trim() : ''
    if (!id) continue
    const titleRaw = block['dcs:title']?.trim()
    out.push({ id, title: titleRaw ?? id })
  }
  return out
}

/**
 * Attribute a finding to a clause only when its message (or path/field IRI text)
 * contains exactly one known clause @id. Mentions of zero or multiple ids stay
 * unassigned — same conservative rule as party DID attribution.
 */
export function partitionFindingsByClause(
  findings: ClauseFindingInput[],
  clauses: { id: string; title: string }[],
): { byClause: Map<string, ClauseFindingInput[]>; unassigned: ClauseFindingInput[] } {
  const byClause = new Map<string, ClauseFindingInput[]>()
  for (const c of clauses) byClause.set(c.id, [])
  const unassigned: ClauseFindingInput[] = []

  for (const finding of findings) {
    const haystack = [finding.message, finding.path, finding.field_iri]
      .filter((v): v is string => !!v?.trim())
      .join('\n')
      .toLowerCase()
    const matched = clauses.filter((c) => haystack.includes(c.id.toLowerCase()))
    if (matched.length === 1) {
      byClause.get(matched[0]!.id)!.push(finding)
      continue
    }
    unassigned.push(finding)
  }
  return { byClause, unassigned }
}

function clauseAssessment(findings: ClauseFindingInput[]): string {
  if (!findings.length) {
    return 'No content-policy findings attributable to this clause'
  }
  const failed = findings.some((f) => {
    const r = (f.result ?? f.severity ?? '').toUpperCase()
    return r === 'FAILED' || r === 'ERROR' || r === 'VIOLATION'
  })
  if (failed) return 'Clause-linked content-policy findings report defects'
  const review = findings.some((f) => (f.result ?? '').toUpperCase() === 'REVIEW')
  if (review) return 'Clause-linked content-policy findings need review'
  return 'Clause-linked content-policy findings recorded'
}

export function buildClauseComplianceReport(input: {
  clauses: { id: string; title: string }[]
  findings: ClauseFindingInput[]
  clausesAvailable: boolean
  findingsAvailable: boolean
  clausesUnavailableReason?: string | null
  findingsUnavailableReason?: string | null
}): ClauseComplianceReport {
  const { byClause, unassigned } = partitionFindingsByClause(input.findings, input.clauses)
  const sections: ClauseComplianceSection[] = input.clauses.map((c) => {
    const findings = byClause.get(c.id) ?? []
    return {
      clause_id: c.id,
      title: c.title,
      findings,
      non_conformance_summary: findings.length
        ? findings.map((f) => f.message).join('; ')
        : 'No clause-specific findings',
      assessment: clauseAssessment(findings),
    }
  })

  return {
    scope_note:
      'Clause sections list dcs:Clause blocks from the contract document. Findings attach only when they unambiguously reference a clause @id.',
    clauses: sections,
    unassigned_findings: unassigned,
    clauses_available: input.clausesAvailable,
    findings_available: input.findingsAvailable,
    clauses_unavailable_reason: input.clausesUnavailableReason ?? null,
    findings_unavailable_reason: input.findingsUnavailableReason ?? null,
  }
}

/** Pull content-policy findings from a PAC /pac/audit timeline (when present). */
export function contentFindingsFromPacTimeline(timeline: unknown): ClauseFindingInput[] {
  if (!Array.isArray(timeline)) return []
  const out: ClauseFindingInput[] = []
  for (const entry of timeline) {
    if (!entry || typeof entry !== 'object') continue
    const e = entry as Record<string, unknown>
    const eventType = typeof e.event_type === 'string' ? e.event_type : ''
    if (eventType !== 'CONTRACT_CONTENT_POLICY_AUDIT_FINDING') continue
    let data = e.event_data
    if (typeof data === 'string') {
      try {
        data = JSON.parse(data) as unknown
      } catch {
        continue
      }
    }
    if (!data || typeof data !== 'object') continue
    const d = data as Record<string, unknown>
    const message = typeof d.message === 'string' ? d.message.trim() : ''
    if (!message) continue
    out.push({
      rule_id: typeof d.ruleId === 'string' ? d.ruleId : undefined,
      severity: typeof d.severity === 'string' ? d.severity : undefined,
      message,
      path: typeof d.path === 'string' ? d.path : undefined,
      field_iri: typeof d.fieldIri === 'string' ? d.fieldIri : undefined,
      result: typeof e.result === 'string' ? e.result : undefined,
    })
  }
  return out
}
