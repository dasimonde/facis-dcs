import type { ContractTemplateState } from "@/types/contract-template-state"
import type { SubTemplateSnapshot } from "@/models/contract-template"
import type {
  DocumentOutline,
  DocumentBlock,
  SemanticCondition,
  MetaData,
  TemplateTypeValue,
  DocumentBlockType,
} from "@template-repository/models/contract-templace"

interface TemplateDraftState {
  did: string | null
  name: string
  description: string
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
}

export interface AddBlockOptions {
  addToOutline?: boolean
}

export type { TemplateDraftState }