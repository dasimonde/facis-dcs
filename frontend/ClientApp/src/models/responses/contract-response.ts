import type { Contract, ContractChangeRequest, ContractDeploymentKpi, ExpirationPolicy } from '../contract/contract'
import type { ContractApprovalTask } from '../contract/contract-approval-task'
import type { ContractData } from '../contract/contract-data'
import type { ContractEvent } from '../contract/contract-event'
import type { ContractNegotiation } from '../contract/contract-negotiation'
import type { ContractNegotiationTask } from '../contract/contract-negotiation-task'
import type { ContractResponsible } from '../contract/contract-responsible'
import type { ContractReviewTask } from '../contract/contract-review-task'
import type { ContractTemplate } from '../contract-template/contract-template'
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
  /**
   * Peer-facing lifecycle inferred by the backend (ADR-13): one of
   * ExtrinsicLifecycle, or a lowercased off-ramp state. Only 'executed' claims
   * every declared signature is collected.
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
  /** The data of that contract */
  contract_data: ContractData
  negotiations: ContractNegotiation[]
  /** KPI reports received via deployment callback, each with the target system's verdict (DCS-FR-CWE-31, DCS-FR-CWE-09, ADR-33) */
  kpis?: ContractDeploymentKpi[]
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

export interface ContractOfferAcceptResponse {
  did: string
}

export interface ContractNegotiationResponse {
  did: string
}

export interface ContractNegotiationRespondResponse {
  id: string
}

export interface ContractNegotiationDraftResponse {
  did: string
  /** The staged change request; absent when no draft is stored. */
  change_request?: ContractChangeRequest
  updated_at?: string
}

export interface ContractApproveResponse {
  did: string
}

export interface ContractRejectResponse {
  did: string
}

export interface ContractWithdrawResponse {
  did: string
}

export interface ContractRenewResponse {
  /** The newly minted renewal contract. */
  did: string
  renews_did: string
  renews_contract_version: number
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

/** A configured Contract Target System deployments may be dispatched to
 *  (ADR-25). Disabled entries stay referenceable so a contract naming one keeps
 *  a readable destination, but dispatch to them is refused. */
export interface ContractTarget {
  id: string
  name: string
  url: string
  description?: string
  enabled: boolean
  created_at?: string
  updated_at?: string
  /** The OAuth2 client this target authenticates its callbacks as (ADR-27).
   *  Absent until a credential has been issued. */
  oauth_client_id?: string
  secret_issued_at?: string
}

/** A freshly issued machine credential. The secret is returned by this response
 *  and by no other: Hydra keeps only a hash, so it cannot be read back and must
 *  be rotated if lost (ADR-27). */
export interface MachineCredential {
  client_id: string
  client_secret: string
  token_url?: string
  issued_at?: string
}

/** A registered non-human caller: an SRS Table 5 System User reaching DCS over
 *  its API (ADR-27). */
export interface MachineIdentity {
  id: string
  name: string
  oauth_client_id: string
  participant_did: string
  roles: string[]
  description?: string
  enabled: boolean
  secret_issued_at?: string
  created_at?: string
  updated_at?: string
}

export interface MachineIdentityCreateResponse {
  identity: MachineIdentity
  credential: MachineCredential
}
