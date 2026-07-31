import type { SemanticConditionValue } from '@/models/contract/contract-data'

export interface ContractContentValuesState {
  semanticConditionValues: SemanticConditionValue[]
}

export type SemanticConditionValueSetter =
  | ((blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) => void)
  | null
