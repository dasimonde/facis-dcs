import type { ContractTemplateState } from '@/types/contract-template-state'
import type { ContractTemplateData } from './contract-template'
import type { ContractTemplateActionFlag } from '@/types/contract-template-action-flag'
import type { ComponentType } from '@/types/component-type'

export interface ContractTemplateCreateEvent {
  did: string
  created_by: string
  updated_at: string
  name: string
  description: string
  template_data: ContractTemplateData
  occurred_at: string
}

export interface ContractTemplateCopyEvent {
  copy_did: string
  new_did: string
  copied_by: string
  occurred_at: string
}

export interface ContractTemplateSubmitEvent {
  did: string
  document_number?: string
  version: number
  previous_state: ContractTemplateState
  new_state: ContractTemplateState
  submitted_by: string
  action_flag: ContractTemplateActionFlag
  comments?: string[]
  occurred_at: string
}

export interface ContractTemplateApproveEvent {
  did: string
  document_number?: string
  version: number
  approved_by: string
  decision_notes?: string[]
  occurred_at: string
}

export interface ContractTemplateRejectEvent {
  did: string
  document_number?: string
  version: string
  rejected_by: string
  reason: string
  occurred_at: string
}

export interface ContractTemplateVerifyEvent {
  did: string
  document_number?: string
  version: number
  verified_by: string
  occurred_at: string
}

export interface ContractTemplateUpdateEvent {
  did: string
  updated_at: string
  old_document_number?: string
  new_document_number?: string
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_template_data?: ContractTemplateData
  new_template_data?: ContractTemplateData
  occurred_at: string
}

export interface ContractTemplateUpdateManageEvent {
  did: string
  updated_at: string
  old_document_number?: string
  new_document_number?: string
  old_state?: ContractTemplateState
  new_state?: ContractTemplateState
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_template_data?: ContractTemplateData
  new_template_data?: ContractTemplateData
  occurred_at: string
}

export interface ContractTemplateSearchEvent {
  retrieved_by: string
  document_number?: string
  version: number
  occurred_at: string
}

export interface ContractTemplateRetrieveAllEvent {
  retrieved_by: string
  occurred_at: string
}

export interface ContractTemplateRetrieveByIDEvent {
  did: string
  document_number?: string
  version: number
  retrieved_by: string
  occurred_at: string
}

export interface ContractTemplateArchiveEvent {
  did: string
  document_number?: string
  version: number
  archived_by: string
  occurred_at: string
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
}

export interface ContractTemplatePublishEvent {
  did: string
  document_number?: string
  version: number
  published_by: string
  occurred_at: string
}

export interface ContractTemplateAuditEvent {
  did: string
  audited_by: string
  occurred_at: string
  component_type: ComponentType
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
