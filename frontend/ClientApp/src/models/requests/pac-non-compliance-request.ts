export interface PACIncidentFindingRequest {
  risk_type: string
  detail: string
}

export interface PACIncidentReportRequest {
  contract_did?: string
  template_did?: string
  findings: PACIncidentFindingRequest[]
}
