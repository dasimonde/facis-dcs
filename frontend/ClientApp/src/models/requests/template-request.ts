import type { DcsTemplateData } from '../dcs-jsonld'
import type { TemplateComplianceStatus, TemplateLevel } from '@/models/contract-template/contract-template'
import type { ContractTemplateActionFlag } from '@/types/contract-template-action-flag'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { PartyType } from '@/types/party-type'
import type { TemplateType } from '@/types/template-type'

export interface ContractTemplateCreateRequest {
  party_type: PartyType
  jurisdiction?: string
  template_level?: TemplateLevel
  required_templates?: string[]
  industry?: string
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
  party_type?: PartyType
  jurisdiction?: string
  template_level?: TemplateLevel
  required_templates?: string[]
  industry?: string
  did: string
  updated_at: string
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: DcsTemplateData
}

export interface ContractTemplateUpdateManageRequest {
  party_type?: PartyType
  jurisdiction?: string
  template_level?: TemplateLevel
  required_templates?: string[]
  industry?: string
  did: string
  updated_at: string
  template_type?: TemplateType
  name?: string
  description?: string
  /** The template data of the contract template */
  template_data?: DcsTemplateData
}

export interface ContractTemplateSearchRequest {
  party_type?: PartyType
  jurisdiction?: string
  industry?: string
  compliance_status?: TemplateComplianceStatus
  offset?: number
  limit?: number
  did?: string
  version?: number
  template_type?: TemplateType
  state?: ContractTemplateState
  name?: string
  description?: string
  created_by?: string
  created_from?: string
  created_to?: string
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
  /** DCS-IR-TR-07: why the template is deprecated or deleted */
  justification: string
}

export interface ContractTemplateRegisterRequest {
  did: string
  version?: number
}

export interface ContractTemplateAuditRequest {
  did: string
  updated_at?: string
  justification: string
}

export interface ContractTemplatePublishRequest {
  did: string
  updated_at: string
}
