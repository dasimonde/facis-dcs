import type { DcsContractData } from '../dcs-jsonld'

export type ContractData = DcsContractData

export interface SemanticConditionValue {
  /** Block ID from top-level template_data.documentBlocks */
  blockId: string
  conditionId: string
  parameterName: string
  parameterValue?: string | number | boolean
}
