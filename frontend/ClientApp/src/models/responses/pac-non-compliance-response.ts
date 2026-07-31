export interface PACComplianceRisk {
  did: string
  risk_type: string
  detail: string
  detected_at: string
}

export interface PACMonitorResponse {
  checked_at: string
  risks: PACComplianceRisk[]
}
