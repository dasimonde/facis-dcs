<script setup lang="ts">
import type { FilterStore } from '@/models/stores/filter-store'
import {
  useApprovalTaskStateFilterStore,
  useNegotiationTaskStateFilterStore,
  useContractStateFilterStore,
  useReviewTaskStateFilterStore,
  useTemplateStateFilterStore,
} from '@/stores/state-filter-store'
import type { ApprovalTaskState } from '@/types/approval-task-state'
import type { ContractState } from '@/types/contract-state'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { NegotiationTaskState } from '@/types/negotiation-task-state'
import type { ReviewTaskState } from '@/types/review-task-state'
import { computed, ref } from 'vue'

const storeMap = {
  templates: useTemplateStateFilterStore,
  contracts: useContractStateFilterStore,
  reviewTasks: useReviewTaskStateFilterStore,
  approvalTasks: useApprovalTaskStateFilterStore,
  negotiationTasks: useNegotiationTaskStateFilterStore,
} as const

type StoreType = keyof typeof storeMap

interface FilterMap {
  templates: ContractTemplateState
  contracts: ContractState
  reviewTasks: ReviewTaskState
  approvalTasks: ApprovalTaskState
  negotiationTasks: NegotiationTaskState
}

const props = defineProps<{
  filters: FilterMap[StoreType][]
  label: string
  storeType: StoreType
  disabled?: boolean
}>()

const filterStore = storeMap[props.storeType]() as unknown as FilterStore<FilterMap[StoreType]>

const showAll = ref(true)

const activeFilters = computed(() => {
  return props.filters.filter((filter) => filterStore.hasFilter(filter))
})

const inactiveFilters = computed(() => {
  return showAll.value ? props.filters.filter((filter) => !filterStore.hasFilter(filter)) : []
})

const shownFilters = computed(() => {
  return [...activeFilters.value, ...inactiveFilters.value]
})

const hasFilters = computed(() => {
  return activeFilters.value.length > 0
})

const setFilter = (stateFilter: FilterMap[typeof props.storeType]) => {
  if (filterStore.hasFilter(stateFilter)) {
    filterStore.removeFilter(stateFilter)
    showAll.value = !hasFilters.value
  } else {
    filterStore.setFilter(stateFilter)
    showAll.value = false
  }
}

const isSelected = (type: FilterMap[typeof props.storeType]) => {
  return filterStore.hasFilter(type)
}
</script>

<template>
  <button
    id="popover-btn"
    popovertarget="filter-popover"
    class="select m-2 w-fit gap-2 select-secondary"
    :class="{ 'btn-disabled': disabled }"
    :disabled="!!disabled"
  >
    Filter
  </button>
  <ul id="filter-popover" popover class="menu dropdown rounded-box rounded-md bg-base-300 shadow-sm">
    <li class="pointer-events-none menu-title">
      <label class="label">{{ label }}</label>
    </li>
    <ul>
      <li
        v-for="filter in shownFilters"
        :key="filter"
        class="flex justify-between transition-colors"
        @click="setFilter(filter)"
      >
        <label class="label flex-1" :class="{ 'mt-1 bg-primary text-primary-content': isSelected(filter) }">
          {{ filter }}
        </label>
      </li>
      <li v-if="hasFilters" class="w-full border-t border-base-300 px-4 py-2 text-sm opacity-60">
        <label class="link cursor-pointer" @click="showAll = !showAll">
          <div v-if="!showAll">See all</div>
          <div v-else>See less</div>
        </label>
      </li>
    </ul>
  </ul>
</template>

<style scoped>
#popover-btn {
  anchor-name: --anchor-filter-popover;
}

#filter-popover {
  position-anchor: --anchor-filter-popover;
}
</style>
