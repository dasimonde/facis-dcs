import { defineStore } from 'pinia'
import type { ContractContentValuesState } from '../models/contract-content-values-store'
import type { SemanticConditionValue } from '@/models/contract-data'

const storeId = 'contractContentValues'
const SEPARATOR = '::'
const defaultState: Readonly<ContractContentValuesState> = {
  semanticConditionValues: [],
}

export const useContractContentValuesStore = defineStore(storeId, {
  state: (): ContractContentValuesState => getInitialState(),
  actions: {
    setSemanticConditionValue(payload: SemanticConditionValue) {
      const idx = this.semanticConditionValues.findIndex(
        (item) =>
          item.blockId === payload.blockId &&
          item.conditionId === payload.conditionId &&
          item.parameterName === payload.parameterName,
      )
      if (idx >= 0) {
        this.semanticConditionValues[idx] = { ...this.semanticConditionValues[idx], ...payload }
        return
      }
      this.semanticConditionValues.push(payload)
    },
    removeSemanticConditionValues(valuesToRemove: SemanticConditionValue[]) {
      if (valuesToRemove.length === 0) return
      const removeKeys = new Set(valuesToRemove.map(buildConditionValueKey))
      this.semanticConditionValues = this.semanticConditionValues.filter(
        (conditionValue) => !removeKeys.has(buildConditionValueKey(conditionValue)),
      )
    },
    reset(overrides?: Partial<ContractContentValuesState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

function getInitialState(): ContractContentValuesState {
  return {
    ...defaultState,
  }
}

function buildConditionValueKey(value: SemanticConditionValue): string {
  return [value.blockId, value.conditionId, value.parameterName, String(value.parameterValue)].join(SEPARATOR)
}
