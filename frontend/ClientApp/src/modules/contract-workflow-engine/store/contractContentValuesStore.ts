import { defineStore } from 'pinia'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { ContractContentValuesState } from '@contract-workflow-engine/models/contract-content-values-store'

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
      const nextState = getInitialState()
      if (overrides) {
        Object.assign(nextState, overrides)
        if (overrides.semanticConditionValues) {
          nextState.semanticConditionValues = overrides.semanticConditionValues.map((value) => ({ ...value }))
        }
      }
      Object.assign(this, nextState)
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
