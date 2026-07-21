import type { Contract, ContractDeploymentKpi, ExpirationPolicy } from '../contract/contract'
import type { ContractApprovalTask } from '../contract/contract-approval-task'
import type { ContractEvent } from '../contract/contract-event'
import type { ContractNegotiation } from '../contract/contract-negotiation'
import type { ContractNegotiationTask } from '../contract/contract-negotiation-task'
import type { ContractResponsible } from '../contract/contract-responsible'
import type { ContractReviewTask } from '../contract/contract-review-task'
import type { ContractData } from '../contract-data'
import type { ContractTemplate } from '../contract-template'
import type { ComponentType } from '@/types/component-type'
import type { ContractEventType } from '@/types/contract-event-type'
import type { ContractState } from '@/types/contract-state'

export interface ContractCreateResponse {
  did: string
}

export interface ContractUpdateResponse {
  did: string
}

export interface ContractSubmitResponse {
  did: string
  current_state: ContractState
}

export type ApprovedContractTemplateRetrieveResponse = ContractTemplate[]

export interface ContractRetrieveResponse {
  contracts: Contract[]
  review_tasks: ContractReviewTask[]
  approval_tasks: ContractApprovalTask[]
  negotiation_tasks: ContractNegotiationTask[]
}

export interface ContractRetrieveByIdResponse {
  did: string
  contract_version: number
  state: ContractState
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
  /** The data of that contract */
  contract_data: ContractData
  negotiations: ContractNegotiation[]
  /** KPI values reported via deployment callback (DCS-FR-CWE-31, DCS-FR-CWE-09) */
  kpis?: ContractDeploymentKpi[]
  /** Metric names whose latest reported value violates its contractual SLA threshold */
  kpi_violations?: string[]
}

export interface ContractDeployResponse {
  did: string
  contract_version: number
  content_hash: string
  timestamp: string
  correlation_id: string
  payload: unknown
}

export interface ContractOfferResponse {
  did: string
}

export interface ContractReviewResponse {
  did: string
}

interface ContractSearchResponseItem {
  did: string
  contract_version: number
  state: ContractState
  name?: string
  description?: string
  start_date?: string
  exp_date?: string
  exp_policy?: ExpirationPolicy
  exp_notice_period?: number
  responsible?: ContractResponsible
  created_at: string
  updated_at: string
}

export type ContractSearchResponse = ContractSearchResponseItem[]

export interface ContractNegotiationResponse {
  did: string
}

export interface ContractNegotiationRespondResponse {
  id: string
}

export interface ContractApproveResponse {
  did: string
}

export interface ContractRejectResponse {
  did: string
}

export interface ContractStoreResponse {
  did: string
}

export interface ContractTerminateResponse {
  did: string
}

export interface ContractAuditResponseItem {
  id: number
  component: ComponentType
  event_type: ContractEventType
  event_data: ContractEvent
  did?: string
  created_at: string
  res_log_pred_cid?: string
}

export type ContractAuditResponse = ContractAuditResponseItem[]

export interface ContractHistoryItem {
  did: string
  contract_version: number
  state: ContractState
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  start_date?: string
  exp_date?: string
  exp_policy?: ExpirationPolicy
  exp_notice_period?: number
  responsible?: unknown
  contract_data?: ContractData
}

export type ContractHistoryResponse = ContractHistoryItem[]
