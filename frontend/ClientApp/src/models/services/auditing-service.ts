import type { AuditReportRequest, AuditRequest } from '@/models/requests/auditing-request'
import type { AuditResponse } from '@/models/responses/auditing-response'

export interface AuditReportArtifact {
  bytes: ArrayBuffer
  contentType: string
  filename: string
}

export interface AuditingService {
  audit: (request: AuditRequest) => Promise<AuditResponse>
  report: (request: AuditReportRequest) => Promise<AuditReportArtifact>
}
