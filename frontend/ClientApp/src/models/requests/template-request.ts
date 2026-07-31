import type { DcsTemplateData } from '../dcs-jsonld'
import type { ContractTemplateActionFlag } from '@/types/contract-template-action-flag'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { TemplateType } from '@/types/template-type'

export interface ContractTemplateCreateRequest {
  template_type: TemplateType
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: DcsTemplateData
}

export interface ContractTemplateCopyRequest {
  did: string
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
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: DcsTemplateData
}

export interface ContractTemplateUpdateManageRequest {
  did: string
  updated_at: string
  template_type?: TemplateType
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: DcsTemplateData
}

export interface ContractTemplateSearchRequest {
  offset?: number
  limit?: number
  did?: string
  version?: number
  template_type?: TemplateType
  state?: ContractTemplateState
  name?: string
  description?: string
  filter?: string
}

export interface ContractTemplateRetrieveRequest {
  offset?: number
  limit?: number
}

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
  version?: number
}

export interface ContractTemplateAuditRequest {
  did: string
  updated_at: string
}

export interface ContractTemplatePublishRequest {
  did: string
  updated_at: string
}
