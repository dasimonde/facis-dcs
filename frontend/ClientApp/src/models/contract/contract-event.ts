import type { ContractChangeRequest } from './contract'
import type { ContractData } from './contract-data'
import type { ContractResponsible } from './contract-responsible'
import type { ComponentType } from '@/types/component-type'
import type { ContractActionFlag } from '@/types/contract-action-flag'
import type { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'

export interface ContractCreateEvent {
  did: string
  holder_did: string
  template_did: string
  created_by: string
  name: string
  description: string
  contract_data: ContractData
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractUpdateEvent {
  did: string
  holder_did: string
  updated_by: string
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_contract_data?: ContractData
  new_contract_data?: ContractData
  occurred_at: string
  old_exp_date?: string
  new_exp_date?: string
  old_exp_policy?: string
  new_exp_policy?: string
  old_exp_notice_period?: number
  new_exp_notice_period?: number
  old_start_date?: string
  new_start_date?: string
  user_roles: UserRole[]
}

export interface ContractSubmitEvent {
  did: string
  holder_did: string
  previous_state: ContractState
  new_state: ContractState
  submitted_by: string
  occurred_at: string
  contract_version: number
  action_flag: ContractActionFlag
  comments: string[]
  responsible?: ContractResponsible
  user_roles: UserRole[]
}

export interface ContractRetrieveByIDEvent {
  did: string
  holder_did: string
  retrieved_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractRetrieveHistoryByDIDEvent {
  did: string
  holder_did: string
  retrieved_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractRetrieveAllEvent {
  holder_did: string
  retrieved_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractVerifyEvent {
  did: string
  holder_did: string
  contract_version: number
  verified_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractNegotiationEvent {
  did: string
  holder_did: string
  contract_version: number
  change_request?: ContractChangeRequest
  negotiated_by: string
  occurred_at: string
  negotiators: string[]
  user_roles: UserRole[]
}

export interface ContractAcceptNegotiationEvent {
  did: string
  holder_did: string
  contract_version: number
  accepted_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractRejectNegotiationEvent {
  did: string
  holder_did: string
  contract_version: number
  rejected_by: string
  rejection_reason?: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractApproveEvent {
  did: string
  holder_did: string
  contract_version: number
  approved_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractRejectEvent {
  did: string
  holder_did: string
  contract_version: string
  rejected_by: string
  reason: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractTerminateEvent {
  did: string
  holder_did: string
  contract_version: string
  reason: string
  terminated_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractRecordEvidenceEvent {
  did: string
  holder_did: string
  contract_version: string
  recorded_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractAuditEvent {
  did: string
  holder_did: string
  audited_by: string
  occurred_at: string
  component_type: ComponentType
  user_roles: UserRole[]
}

export interface ContractReviewEvent {
  did: string
  holder_did: string
  reviewed_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractIncreaseContractVersionEvent {
  did: string
  holder_did: string
  old_contract_version: number
  new_contract_versiom: number
  submitted_by: string
  occurred_at: string
  user_roles: UserRole[]
}

// Emitted when folding a negotiation round discarded the content of a change
// request that stands at ACCEPTED, because a later accepted request set the
// same fields. Without it the trail shows the acceptance and nothing else.
export interface ContractNegotiationChangeSupersededEvent {
  did: string
  holder_did: string
  contract_version: number
  merged_contract_version: number
  superseded: {
    negotiation_id: string
    superseded_by: string
    fields: string[]
  }[]
  submitted_by: string
  occurred_at: string
  user_roles: UserRole[]
}

export type ContractEvent =
  | ContractCreateEvent
  | ContractUpdateEvent
  | ContractSubmitEvent
  | ContractRetrieveByIDEvent
  | ContractRetrieveHistoryByDIDEvent
  | ContractRetrieveAllEvent
  | ContractVerifyEvent
  | ContractNegotiationEvent
  | ContractAcceptNegotiationEvent
  | ContractRejectNegotiationEvent
  | ContractApproveEvent
  | ContractRejectEvent
  | ContractTerminateEvent
  | ContractRecordEvidenceEvent
  | ContractAuditEvent
  | ContractReviewEvent
  | ContractIncreaseContractVersionEvent
  | ContractNegotiationChangeSupersededEvent
