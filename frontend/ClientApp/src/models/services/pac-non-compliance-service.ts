import type { PACIncidentReportRequest } from '@/models/requests/pac-non-compliance-request'
import type { PACCase, PACMonitorResponse, PACWorkflowGateRun } from '@/models/responses/pac-non-compliance-response'

export interface PACNonComplianceService {
  monitor: (justification?: string) => Promise<PACMonitorResponse>
  reportIncident: (request: PACIncidentReportRequest) => Promise<string | undefined>
  exportCaseFile: (caseId: string, justification: string) => Promise<Blob>
  exportPartyReport: (did: string) => Promise<Blob>
  cases: () => Promise<PACCase[]>
  workflowGateRuns: () => Promise<PACWorkflowGateRun[]>
  decideWorkflowGateReview: (runId: string, decision: 'approve' | 'reject', justification: string) => Promise<void>
}
