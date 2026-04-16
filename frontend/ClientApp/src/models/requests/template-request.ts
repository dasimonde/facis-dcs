import type { TemplateType } from '@/types/template-type'
import type { ContractTemplateActionFlag } from '../../types/action-flag'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { ContractTemplateData } from '../contract-template'

export interface ContractTemplateCreateRequest {
  template_type: TemplateType
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: ContractTemplateData
}

export interface ContractTemplateSubmitRequest {
  did: string
  updated_at: string
  reviewers?: string[]
  approver?: string
  forward_to?: ContractTemplateActionFlag
  comments?: string[]
}

export interface ContractTemplateUpdateRequest {
  did: string
  updated_at: string
  document_number?: string
  version?: number
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: ContractTemplateData
}

export interface ContractTemplateUpdateManageRequest {
  did: string
  state?: ContractTemplateState
  updated_at: string
  document_number?: string
  version?: number
  template_type?: TemplateType
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: ContractTemplateData
}

export interface ContractTemplateSearchRequest {
  did?: string
  document_number?: string
  version?: number
  template_type?: TemplateType
  state?: ContractTemplateState
  name?: string
  description?: string
  filter?: string
}

export interface ContractTemplateRetrieveRequest {}

export interface ContractTemplateRetrieveByIdRequest {
  did: string
}

export interface ContractTemplateApproveRequest {
  did: string
  updated_at: string
  decision_notes?: string[]
}

export interface ContractTemplateRejectRequest {
  did: string
  updated_at: string
  /** Reason for rejecting the contract template */
  reason: string
}

export interface ContractTemplateVerifyRequest {
  did: string
}

export interface ContractTemplateArchiveRequest {
  did: string
  updated_at: string
}

export interface ContractTemplateRegisterRequest {
  did: string
}

export interface ContractTemplateAuditRequest {
  did: string
}
