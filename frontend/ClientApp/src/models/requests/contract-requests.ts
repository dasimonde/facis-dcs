import type { ContractActionFlag } from '@/types/action-flag'
import type { ContractState } from '@/types/contract-state'
import type { NegotiationActionFlag } from '@/types/negotiation-action-flag'

export interface ContractCreateRequest {
  did: string
}

export interface ContractUpdateRequest {
  did: string
  updated_at: string
  expiration_date?: string
  contract_version?: number
  name?: string
  description?: string
  /** The data of the contract */
  contract_data?: unknown
}

export interface ContractSubmitRequest {
  did: string
  updated_at: string
  forward_to?: ContractActionFlag
  comments?: string[]
  reviewers?: string[]
  approver?: string
  negotiators?: string[]
}

export interface ContractRetrieveRequest {}

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
  change_request: unknown
}

export interface ContractNegotiationRespondRequest {
  id: string
  action_flag: NegotiationActionFlag
  responded_by: string
  RejectionReason?: string
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
  /** Reason for terminating the contract */
  reason: string
}

export interface ContractAuditRequest {
  did: string
  updated_at: string
}
