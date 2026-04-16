import type { FilterStore } from '@/models/stores/filter-store'
import type { ApprovalTaskState } from '@/types/approval-task-state'
import type { ContractState } from '@/types/contract-state'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { NegotiationTaskState } from '@/types/negotiation-task-state'
import type { ReviewTaskState } from '@/types/review-task-state'
import { defineStore } from 'pinia'
import { computed, type Ref, ref } from 'vue'

function createFilterStore<T>(storeId: string) {
  return defineStore(storeId, () => {
    const stateFilters: Ref<Set<T>> = ref(new Set()) as Ref<Set<T>>

    const hasFilters = computed(() => stateFilters.value.size > 0)

    function hasFilter(filter: T) {
      return stateFilters.value.has(filter)
    }

    function setFilter(filter: T) {
      stateFilters.value.add(filter)
    }

    function removeFilter(filter: T) {
      stateFilters.value.delete(filter)
    }

    function reset() {
      stateFilters.value.clear()
    }

    return {
      stateFilters,
      hasFilters,
      hasFilter,
      setFilter,
      removeFilter,
      reset,
    } satisfies FilterStore<T>
  })
}

export const useTemplateStateFilterStore = createFilterStore<ContractTemplateState>('templateStateFilter')
export const useContractStateFilterStore = createFilterStore<ContractState>('contractStateFilter')
export const useReviewTaskStateFilterStore = createFilterStore<ReviewTaskState>('reviewTaskStateFilter')
export const useApprovalTaskStateFilterStore = createFilterStore<ApprovalTaskState>('approvalTaskStateFilter')
export const useNegotiationTaskStateFilterStore =
  createFilterStore<NegotiationTaskState>('negotiationTaskStateFilter')
