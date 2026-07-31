import type { PACIncidentReportRequest } from '@/models/requests/pac-non-compliance-request'
import type { PACMonitorResponse } from '@/models/responses/pac-non-compliance-response'

export interface PACNonComplianceService {
  monitor: () => Promise<PACMonitorResponse>
  reportIncident: (request: PACIncidentReportRequest) => Promise<void>
}
