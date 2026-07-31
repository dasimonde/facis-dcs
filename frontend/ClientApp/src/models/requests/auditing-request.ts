export type AuditScope = 'templates' | 'contracts' | 'signatures' | 'archive'
export type AuditReportFormat = 'json' | 'csv' | 'pdf'

export interface AuditRequest {
  scope: AuditScope
  did?: string
  justification: string
}

export interface AuditReportRequest extends AuditRequest {
  format?: AuditReportFormat
  did?: string
}
