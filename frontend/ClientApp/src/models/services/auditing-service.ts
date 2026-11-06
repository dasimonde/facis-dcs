import type { AccessAttemptsRequest, AuditReportRequest, AuditRequest } from '@/models/requests/auditing-request'
import type { AccessAttemptsResponse, AuditResponse, CheckpointProof } from '@/models/responses/auditing-response'

export interface AuditReportArtifact {
  bytes: ArrayBuffer
  contentType: string
  filename: string
}

export interface AuditingService {
  accessAttempts: (request: AccessAttemptsRequest) => Promise<AccessAttemptsResponse>
  audit: (request: AuditRequest) => Promise<AuditResponse>
  report: (request: AuditReportRequest) => Promise<AuditReportArtifact>
  checkpointProof: (entryCid: string) => Promise<CheckpointProof>
}
