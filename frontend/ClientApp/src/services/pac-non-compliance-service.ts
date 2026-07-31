import http from '@/api/http'
import type { PACIncidentReportRequest } from '@/models/requests/pac-non-compliance-request'
import type { PACMonitorResponse } from '@/models/responses/pac-non-compliance-response'
import type { PACNonComplianceService } from '@/models/services/pac-non-compliance-service'

export const pacNonComplianceService: PACNonComplianceService = {
  async monitor() {
    return http.get<PACMonitorResponse>('/pac/monitor').then((res) => res.data)
  },

  async reportIncident(request: PACIncidentReportRequest) {
    await http.post('/pac/report', request)
  },
}
