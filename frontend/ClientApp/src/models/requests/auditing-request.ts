// PROCESS_AUDIT_AND_COMPLIANCE: the compliance component's own trail, with the
// configuration attestation records of each backend start (DCS-NFR-SEC-04).
export type AuditScope = 'templates' | 'contracts' | 'signatures' | 'archive' | 'PROCESS_AUDIT_AND_COMPLIANCE'
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

export interface AccessAttemptsRequest {
  justification: string
  limit?: number
  cursor?: string
}
