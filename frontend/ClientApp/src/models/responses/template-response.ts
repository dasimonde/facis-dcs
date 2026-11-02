import type {
  PartialContractTemplate,
  TemplateReviewNote,
  TemplateSearchableMetadata,
  TemplateStructure,
} from '../contract-template/contract-template'
import type { ContractTemplateApprovalTask } from '../contract-template/contract-template-approval-task'
import type { ContractTemplateEvent } from '../contract-template/contract-template-event'
import type { ContractTemplateResponsible } from '../contract-template/contract-template-responsible'
import type { ContractTemplateReviewTask } from '../contract-template/contract-template-review-task'
import type { DcsTemplateData } from '../dcs-jsonld'
import type { ComponentType } from '@/types/component-type'
import type { ContractTemplateEventType } from '@/types/contract-template-event-type'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { PartyType } from '@/types/party-type'
import type { TemplateType } from '@/types/template-type'

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

interface ContractTemplateSearchResponseItem extends TemplateSearchableMetadata, TemplateStructure {
  did: string
  version: string
  state: ContractTemplateState
  party_type: PartyType
  template_type: TemplateType
  name?: string
  description?: string
  created_at: string
  created_by: string
  updated_at: string
}

export type ContractTemplateSearchResponse = ContractTemplateSearchResponseItem[]

// One snapshot from GET /template/history/{did}, recorded at a state change.
export interface ContractTemplateHistoryEntry {
  did: string
  version: number
  state: ContractTemplateState
  template_type: TemplateType
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
}

export type ContractTemplateHistoryResponse = ContractTemplateHistoryEntry[]

export interface ContractTemplateRetrieveResponse {
  contract_templates: PartialContractTemplate[]
  review_tasks: ContractTemplateReviewTask[]
  approval_tasks: ContractTemplateApprovalTask[]
}

export interface ContractTemplateRetrieveByIdResponse extends TemplateSearchableMetadata, TemplateStructure {
  did: string
  version: number
  state: ContractTemplateState
  party_type: PartyType
  template_type: TemplateType
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  /** The template data of the contract template */
  template_data: DcsTemplateData
  responsible?: ContractTemplateResponsible
  /** Oldest first; returned to the template roles only */
  review_notes?: TemplateReviewNote[]
}

export interface ContractTemplateApproveResponse {
  did: string
}

export interface ContractTemplateRejectResponse {
  did: string
}

export interface ContractTemplateVerifyResponse {
  did: string
  /** Findings block the review. */
  findings: string[]
  /** Warnings do not block the review. */
  warnings?: string[]
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
}

export type ContractTemplateAuditResponse = ContractTemplateAuditResponseItem[]

export interface ContractTemplatePublishResponse {
  did: string
}

/** DCS-FR-TR-09: the provenance credential of one registered template version (W3C VC). */
export interface TemplateProvenanceCredential {
  version: number
  vc_id: string
  previous_vc_id?: string
  credential: {
    issuer: string
    validFrom?: string
    credentialSubject: {
      template_did: string
      version: number
      template_hash: string
      created_by: string
      reviewed_by?: string[]
      approved_by?: string[]
      registered_by?: string
      registrar_holder_did?: string
      previous_credential?: string
    }
    proof?: Record<string, unknown>
  }
}

export type ContractTemplateProvenanceResponse = TemplateProvenanceCredential[]

export interface ContractTemplatePdfVerifyResponse {
  match: boolean
  jsonld_hash: string
  base_pdf_hash: string
  stored_base_pdf_hash: string
}
