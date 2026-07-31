import type { ContractTemplateData } from './contract-template'
import type { ContractTemplateResponsible } from './contract-template-responsible'
import type { ComponentType } from '@/types/component-type'
import type { ContractTemplateActionFlag } from '@/types/contract-template-action-flag'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { UserRole } from '@/types/user-role'

export interface ContractTemplateCreateEvent {
  did: string
  created_by: string
  updated_at: string
  name: string
  description: string
  template_data: ContractTemplateData
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateCopyEvent {
  copy_did: string
  new_did: string
  copied_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateSubmitEvent {
  did: string
  version: number
  previous_state: ContractTemplateState
  new_state: ContractTemplateState
  submitted_by: string
  action_flag: ContractTemplateActionFlag
  comments?: string[]
  occurred_at: string
  responsible?: ContractTemplateResponsible
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateApproveEvent {
  did: string
  version: number
  approved_by: string
  decision_notes?: string[]
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateRejectEvent {
  did: string
  version: string
  rejected_by: string
  reason: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateVerifyEvent {
  did: string
  version: number
  verified_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateUpdateEvent {
  did: string
  updated_by: string
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_template_data?: ContractTemplateData
  new_template_data?: ContractTemplateData
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateUpdateManageEvent {
  did: string
  updated_at: string
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_template_data?: ContractTemplateData
  new_template_data?: ContractTemplateData
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateSearchEvent {
  retrieved_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateRetrieveAllEvent {
  retrieved_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateRetrieveByIDEvent {
  did: string
  version: number
  retrieved_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateArchiveEvent {
  did: string
  version: number
  archived_by: string
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplateRegisterEvent {
  did: string
  registered_by: string
  updated_at: string
  name?: string
  description?: string
  template_data?: ContractTemplateData
  source_did: string
  source_version: number
  occurred_at: string
  holder_did: string
  user_roles: UserRole[]
}

export interface ContractTemplatePublishEvent {
  did: string
  version: number
  published_by: string
  holder_did: string
  occurred_at: string
  user_roles: UserRole[]
}

export interface ContractTemplateAuditEvent {
  did: string
  audited_by: string
  occurred_at: string
  component_type: ComponentType
  holder_did: string
  user_roles: UserRole[]
}

export type ContractTemplateEvent =
  | ContractTemplateCreateEvent
  | ContractTemplateSubmitEvent
  | ContractTemplateApproveEvent
  | ContractTemplateRejectEvent
  | ContractTemplateVerifyEvent
  | ContractTemplateUpdateEvent
  | ContractTemplateUpdateManageEvent
  | ContractTemplateSearchEvent
  | ContractTemplateRetrieveAllEvent
  | ContractTemplateRetrieveByIDEvent
  | ContractTemplateArchiveEvent
  | ContractTemplateRegisterEvent
  | ContractTemplatePublishEvent
  | ContractTemplateAuditEvent
