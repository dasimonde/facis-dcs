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

/**
 * What the target system concluded about the ODRL rule its report names
 * (ADR-33). 'not_evaluated' is a distinct outcome, neither a breach nor
 * compliance, and never renders as either.
 */
export type KpiVerdict = 'satisfied' | 'violated' | 'not_evaluated'

export interface ContractDeploymentKpi {
  metric: string
  value: string
  observed_at: string
  verdict: KpiVerdict
  /** @id of the ODRL rule the verdict concerns; absent when the target named none */
  rule?: string
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
  /** KPI reports received via deployment callback, each with the target system's verdict (DCS-FR-CWE-31, DCS-FR-CWE-09, ADR-33) */
  kpis?: ContractDeploymentKpi[]
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
