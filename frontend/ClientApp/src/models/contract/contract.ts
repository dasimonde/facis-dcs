import type { ContractData } from './contract-data'
import type { ContractNegotiation } from './contract-negotiation'
import type { ContractResponsible } from './contract-responsible'
import type { ContractState } from '@/types/contract-state'

export const ExpirationPolicy = {
  renewal: 'RENEWAL',
  archiving: 'ARCHIVING',
  termination: 'TERMINATION',
} as const

export type ExpirationPolicy = (typeof ExpirationPolicy)[keyof typeof ExpirationPolicy]

export interface ContractDeploymentKpi {
  metric: string
  value: string
  observed_at: string
  violation?: boolean
}

export interface Contract {
  did: string
  contract_version: number
  state: ContractState
  /**
   * Peer-facing lifecycle inferred by the backend (ADR-13): one of
   * ExtrinsicLifecycle, or a lowercased off-ramp state. Only 'executed' claims
   * every declared signature is collected — the intrinsic SIGNED state does
   * not, it is written on the first signature.
   */
  extrinsic_lifecycle?: string
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  start_date?: string
  exp_date?: string
  exp_notice_period?: number
  exp_policy?: ExpirationPolicy
  responsible?: ContractResponsible
  contract_data?: ContractData
  negotiations?: ContractNegotiation[]
  outdated?: boolean
  latest_template_did?: string
  template_did?: string
  template_version?: number
  template_is_deprecated?: boolean
  parent_contract_did?: string
  /** KPI values reported via deployment callback (DCS-FR-CWE-31, DCS-FR-CWE-09) */
  kpis?: ContractDeploymentKpi[]
  /** Metric names whose latest reported value violates its contractual SLA threshold */
  kpi_violations?: string[]
  /** Registered target system this contract deploys to (ADR-25); absent until designated */
  target_id?: string
  /** Name of that target, so the destination is readable without a second lookup */
  target_name?: string
}

export type ContractChangeRequest = Pick<Contract, 'name' | 'description' | 'exp_notice_period' | 'exp_policy'> & {
  /** A content redline is a complete canonical contract document. The backend
   *  validates and replaces contract_data atomically; nested partial patches
   *  are not part of the negotiation contract. */
  contract_data?: ContractData
}
