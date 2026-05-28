import type { ContractActionFlag } from '@/types/contract-action-flag'
import type { ContractState } from '@/types/contract-state'
import type { NegotiationActionFlag } from '@/types/negotiation-action-flag'
import type { ContractData } from '../contract-data'
import type { ContractChangeRequest, ExpirationPolicy } from '../contract/contract'

export interface ContractCreateRequest {
  did: string
}

export interface ContractUpdateRequest {
  did: string
  updated_at: string
  exp_notice_period?: number
  exp_policy?: ExpirationPolicy
  name?: string
  description?: string
  /** The data of the contract */
  contract_data?: ContractData
}

export interface ContractSubmitRequest {
  did: string
  updated_at: string
  forward_to?: ContractActionFlag
  comments?: string[]
  reviewers?: string[]
  approvers?: string[]
  negotiators?: string[]
}

export type ContractRetrieveRequest = Record<string, unknown>

export interface ContractRetrieveByIdRequest {
  did: string
}

export interface ContractReviewRequest {
  did: string
}

export interface ContractSearchRequest {
  did?: string
  contract_version?: number
  state?: ContractState
  name?: string
  description?: string
  filter?: string
}

export interface ContractNegotiationRequest {
  did: string
  updated_at: string
  negotiated_by: string
  change_request: ContractChangeRequest
}

export interface ContractNegotiationRespondRequest {
  id: string
  did: string
  action_flag: NegotiationActionFlag
  responded_by: string
  rejection_reason?: string
}

export interface ContractApproveRequest {
  did: string
  updated_at: string
}

export interface ContractRejectRequest {
  did: string
  updated_at: string
  /** Reason for rejecting the contract */
  reason: string
}

export interface ContractStoreRequest {
  did: string
  updated_at: string
}

export interface ContractTerminateRequest {
  did: string
  updated_at: string
  /** Reason for terminating the contract */
  reason: string
}

export interface ContractAuditRequest {
  did: string
}

export interface ContractHistoryRetrieveRequest {
  did: string
}
