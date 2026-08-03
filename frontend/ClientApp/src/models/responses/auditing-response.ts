export type AuditFindingCategory = 'violation' | 'inconsistency' | 'compliance_check'

export interface AuditFinding {
  id: number | string
  category: AuditFindingCategory | (string & {})
  title?: string
  description?: string
  component?: string
  status?: string
  did?: string
  object_name?: string
  object_type?: string
  created_at: string
  details?: unknown
}

export type AuditResponse = AuditFinding[]

export interface PACAuditExecutorFinding {
  rule_id: string
  result: 'PASSED' | 'FAILED' | 'REVIEW'
  reason: string
  severity?: string
  evidence_refs?: string[]
}

export interface PACAuditExecutorResponse {
  contract_version: string
  audit_id: string
  correlation_id: string
  scope: string
  resource?: { did: string }
  executor: {
    id: string
    version: string
  }
  executed_at: string
  findings: PACAuditExecutorFinding[]
  receipt?: unknown
}

export interface AuditReportSummary {
  totalEvents: number
  totalChecks: number
  passed: number
  failed: number
  warnings: number
  needsReview: number
}

export interface AuditReportResource {
  did: string
  component: string
  eventCount: number
  findingCount: number
}

export interface AuditReportEvent {
  timestamp: string
  actor?: string
  component: string
  eventType: string
  did?: string
  details?: Record<string, unknown>
}

export interface AuditReportFinding {
  timestamp: string
  component: string
  eventType: string
  did?: string
  ruleId?: string
  title?: string
  severity?: string
  message?: string
  requirement?: string
  actualValue?: unknown
  expectedValue?: unknown
  expectedValues?: unknown[]
  operator?: string
  path?: string
  fieldIri?: string
  ontologyTerm?: string
  actor?: string
}

export interface AuditReport {
  reportId: string
  scope: string
  generatedAt: string
  generatedBy: string
  format: 'json'
  did?: string
  contentHash?: string
  summary: AuditReportSummary
  resources: AuditReportResource[]
  events: AuditReportEvent[]
  findings: AuditReportFinding[]
}

export interface AuditReportDownload {
  reportId: string
  scope: string
  format: 'csv' | 'pdf'
  contentType: string
  filename: string
  encoding: 'utf-8' | 'base64'
  content: string
  contentHash: string
  summary: AuditReportSummary
}

export type AuditReportResponse = AuditReport | AuditReportDownload
