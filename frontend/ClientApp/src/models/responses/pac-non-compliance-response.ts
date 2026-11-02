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

/** A workflow-gate run; a REVIEW run waits for a Compliance Officer decision (DCS-FR-PACM-03). */
export interface PACWorkflowGateRun {
  run_id: string
  contract_did: string
  contract_version: number
  gate: string
  status: string
  findings?: string[]
  created_at?: string
  review?: { status: string; decision?: string; justification?: string; decided_by?: string; decided_at?: string }
}

/** One incident case (DCS-IR-PACM-04, DCS-FR-PACM-05). */
export interface PACCase {
  case_id: string
  did: string
  linked_to: 'contract' | 'template'
  reported_by: string
  reported_at: string
  findings: number
  exported_at?: string
}
