import type { ComponentType } from '@/types/component-type'
import type { ContractTemplateEventType } from '@/types/contract-template-event-type'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { TemplateType } from '@/types/template-type'
import type { ContractTemplateData, PartialContractTemplate } from '../contract-template'
import type { ContractTemplateApprovalTask } from '../contract-template-approval-task'
import type { ContractTemplateEvent } from '../contract-template-event'
import type { ContractTemplateReviewTask } from '../contract-template-review-task'
import type { ContractTemplateResponsiblePersons } from '../contract-template-responsible-persons'

export interface ContractTemplateCreateResponse {
  did: string
}

export interface ContractTemplateCopyResponse {
  did: string
}

export interface ContractTemplateSubmitResponse {
  did: string
}

export interface ContractTemplateUpdateResponse {
  did: string
}

export interface ContractTemplateUpdateManageResponse {
  did: string
}

interface ContractTemplateSearchResponseItem {
  did: string
  document_number?: string
  version: string
  state: ContractTemplateState
  template_type: TemplateType
  name?: string
  description?: string
  created_at: string
  created_by: string
  updated_at: string
  responsible_persons?: ContractTemplateResponsiblePersons
}

export type ContractTemplateSearchResponse = ContractTemplateSearchResponseItem[]

export interface ContractTemplateRetrieveResponse {
  contract_templates: PartialContractTemplate[]
  review_tasks: ContractTemplateReviewTask[]
  approval_tasks: ContractTemplateApprovalTask[]
}

export interface ContractTemplateRetrieveByIdResponse {
  did: string
  document_number?: string
  version: number
  state: ContractTemplateState
  template_type: TemplateType
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  /** The template data of the contract template */
  template_data: ContractTemplateData
  responsible_persons?: ContractTemplateResponsiblePersons
}

export interface ContractTemplateApproveResponse {
  did: string
}

export interface ContractTemplateRejectResponse {
  did: string
}

export interface ContractTemplateVerifyResponse {
  did: string
  findings: string[]
}

export interface ContractTemplateArchiveResponse {
  did: string
}

export interface ContractTemplateRegisterResponse {
  did: string
}

export interface ContractTemplateAuditResponseItem {
  id: number
  component: ComponentType
  event_type: ContractTemplateEventType
  event_data: ContractTemplateEvent
  did?: string
  created_at: string
  res_log_pred_cid?: string
  global_log_pred_cid?: string
}

export type ContractTemplateAuditResponse = ContractTemplateAuditResponseItem[]

export interface ContractTemplatePublishResponse {
  did: string
}
