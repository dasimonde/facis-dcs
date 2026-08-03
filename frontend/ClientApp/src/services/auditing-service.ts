import http from '@/api/http'
import type { AuditReportRequest, AuditRequest } from '@/models/requests/auditing-request'
import type { AuditFinding, AuditResponse, PACAuditExecutorResponse } from '@/models/responses/auditing-response'
import type { AuditingService } from '@/models/services/auditing-service'

const normalizeAuditResponse = (data: PACAuditExecutorResponse): AuditResponse => {
  if (!isExecutorResponse(data)) {
    throw new Error('Invalid external audit executor response')
  }
  return data.findings.map((finding, index) => ({
    id: `${data.audit_id}-${finding.rule_id}-${index}`,
    category: categoryFromResult(finding.result),
    title: finding.rule_id,
    description: finding.reason,
    component: data.executor.id,
    status: finding.result,
    did: data.resource?.did,
    created_at: data.executed_at,
    details: {
      finding,
      audit_id: data.audit_id,
      correlation_id: data.correlation_id,
      contract_version: data.contract_version,
      scope: data.scope,
      executor: data.executor,
      receipt: data.receipt,
    },
  }))
}

function isExecutorResponse(value: unknown): value is PACAuditExecutorResponse {
  return (
    isObjectRecord(value) &&
    typeof value.audit_id === 'string' &&
    isObjectRecord(value.executor) &&
    Array.isArray(value.findings)
  )
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function categoryFromResult(result: PACAuditExecutorResponse['findings'][number]['result']): AuditFinding['category'] {
  if (result === 'FAILED') return 'violation'
  if (result === 'REVIEW') return 'inconsistency'
  return 'compliance_check'
}

export const auditingService: AuditingService = {
  async audit(request: AuditRequest) {
    return http.post<PACAuditExecutorResponse>('/pac/audit', request).then((res) => normalizeAuditResponse(res.data))
  },

  async report(request: AuditReportRequest) {
    return http.get<ArrayBuffer>('/pac/report', { params: request, responseType: 'arraybuffer' }).then((res) => ({
      bytes: res.data,
      contentType: res.headers['content-type'] ?? 'application/octet-stream',
      filename: `audit-report-${request.scope}.${request.format ?? 'json'}`,
    }))
  },
}
