import type {
  DocumentBlock,
  DocumentOutline,
  SemanticCondition,
} from '@/modules/template-repository/models/contract-templace'
import type { TemplateDataVersion } from '@/modules/template-repository/models/template-draft-store'
import type { SubTemplateSnapshot } from './contract-template'

export interface ContractData {
  documentOutline: DocumentOutline
  documentBlocks: DocumentBlock[]
  semanticConditions: SemanticCondition[]
  subTemplateSnapshots: SubTemplateSnapshot[]
  templateDataVersion: TemplateDataVersion
  semanticConditionValues: SemanticConditionValue[]
}

export interface SemanticConditionValue {
  /** Block ID from top-level template_data.documentBlocks */
  blockId: string
  conditionId: string
  parameterName: string
  parameterValue?: string | number
}
