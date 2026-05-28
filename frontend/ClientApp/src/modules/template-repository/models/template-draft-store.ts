import type { ContractTemplateState } from '@/types/contract-template-state'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type {
  DocumentOutline,
  DocumentBlock,
  SemanticCondition,
  MetaData,
  TemplateTypeValue,
  DocumentBlockType,
} from '@template-repository/models/contract-templace'
import type { ContractTemplateResponsiblePersons } from '@/models/contract-template-responsible-persons'

export const TEMPLATE_DATA_VERSIONS = [1] as const
export type TemplateDataVersion = (typeof TEMPLATE_DATA_VERSIONS)[number]

interface TemplateDraftState {
  did: string | null
  name: string
  description: string
  templateDataVersion: TemplateDataVersion
  documentOutline: DocumentOutline
  documentBlocks: DocumentBlock[]
  semanticConditions: SemanticCondition[]
  customMetaData: MetaData[]
  subTemplateSnapshots: SubTemplateSnapshot[]
  templateType: TemplateTypeValue
  state: ContractTemplateState | null
  document_number: string | null
  version: number | null
  updated_at: string | null
  created_by: string
  responsible_persons: ContractTemplateResponsiblePersons | null
  workflow: 'contract' | 'template'
}

/** Payload for adding a new block. */
export interface AddBlockPayload {
  blockType: DocumentBlockType
  text: string
  title?: string
  // #### For Clause ####
  clauseBlockId?: string
  conditionIds?: string[]
  // #### For ApprovedTemplate ####
  templateId?: string
  version?: number
  document_number?: string
  merged_approved_block_id?: string
}

export interface AddBlockOptions {
  addToOutline?: boolean
}

export type SubTemplateReference = Pick<SubTemplateSnapshot, 'did' | 'version' | 'document_number'>

export type { TemplateDraftState }
