import http from '@/api/http'
import type { PACIncidentReportRequest } from '@/models/requests/pac-non-compliance-request'
import type { PACCase, PACMonitorResponse, PACWorkflowGateRun } from '@/models/responses/pac-non-compliance-response'
import type { PACNonComplianceService } from '@/models/services/pac-non-compliance-service'

export const pacNonComplianceService: PACNonComplianceService = {
  // With a justification an unauthorized-access risk names the denied actor (DCS-FR-PACM-04).
  async monitor(justification?: string) {
    return http
      .get<PACMonitorResponse>('/pac/monitor', { params: justification ? { justification } : undefined })
      .then((res) => res.data)
  },

  async reportIncident(request: PACIncidentReportRequest) {
    return http.post<{ case_id?: string }>('/pac/report', request).then((res) => res.data?.case_id)
  },

  async exportCaseFile(caseId: string, justification: string) {
    return http
      .get<Blob>(`/pac/case/${encodeURIComponent(caseId)}`, { params: { justification }, responseType: 'blob' })
      .then((res) => res.data)
  },

  async exportPartyReport(did: string) {
    return http.get<Blob>('/pac/party-report', { params: { did }, responseType: 'blob' }).then((res) => res.data)
  },

  async cases() {
    return http.get<PACCase[]>('/pac/cases').then((res) => res.data ?? [])
  },

  // Without a filter the backend lists the REVIEW runs that wait for a decision.
  async workflowGateRuns() {
    return http.get<PACWorkflowGateRun[]>('/pac/workflow-gates').then((res) => res.data ?? [])
  },

  async decideWorkflowGateReview(runId: string, decision: 'approve' | 'reject', justification: string) {
    await http.post('/pac/workflow-gates/review', { run_id: runId, decision, justification })
  },
}
