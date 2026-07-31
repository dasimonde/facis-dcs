<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef } from 'vue'
import {
  useApprovalTaskStateFilterStore,
  useContractStateFilterStore,
  useNegotiationTaskStateFilterStore,
  useReviewTaskStateFilterStore,
  useTemplateStateFilterStore,
} from '@/stores/state-filter-store'
import type { FilterStore } from '@/models/stores/filter-store'
import type { ApprovalTaskState } from '@/types/approval-task-state'
import type { ContractState } from '@/types/contract-state'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { NegotiationTaskState } from '@/types/negotiation-task-state'
import type { ReviewTaskState } from '@/types/review-task-state'

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
const filterPopover = useTemplateRef('filter-popover')

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

const showInitialFocus = ref(true)

const focusFirstOption = () => {
  void nextTick(() => {
    filterPopover.value?.querySelector<HTMLElement>('a[tabindex="0"]')?.focus()
  })
}

const handlePopoverToggle = (event: ToggleEvent) => {
  if (event.newState === 'closed') {
    showInitialFocus.value = true
  } else if (showInitialFocus.value) {
    focusFirstOption()
  }
}
</script>

<template>
  <button
    id="popover-btn"
    popovertarget="filter-popover"
    class="select-button btn m-2 btn-block w-fit cursor-default justify-between gap-2 border-secondary btn-outline-default"
    :class="{ 'btn-disabled': disabled }"
    :disabled="!!disabled"
  >
    Filter
  </button>
  <ul
    id="filter-popover"
    ref="filter-popover"
    popover
    class="menu dropdown mt-2 rounded-box rounded-md bg-base-300 shadow-sm"
    @toggle="handlePopoverToggle"
  >
    <li class="pointer-events-none menu-title">
      <h1 class="label text-base-content/70">{{ label }}</h1>
    </li>
    <li>
      <ul>
        <li v-for="(filter, index) in shownFilters" :key="filter" class="flex justify-between transition-colors">
          <a
            tabindex="0"
            class="label flex-1 text-base-content/70"
            :class="{
              'mt-1 bg-primary text-primary-content': isSelected(filter),
              'menu-focus': index === 0 && showInitialFocus,
            }"
            @blur="index === 0 ? (showInitialFocus = false) : null"
            @click="setFilter(filter)"
            @keydown.enter="setFilter(filter)"
            @keydown.space.prevent="setFilter(filter)"
          >
            {{ filter }}
          </a>
        </li>
        <li v-if="hasFilters" class="w-full border-t border-base-300 px-4 py-2 text-sm opacity-60">
          <a
            tabindex="0"
            class="link cursor-pointer"
            @click="showAll = !showAll"
            @keydown.enter="showAll = !showAll"
            @keydown.space.prevent="showAll = !showAll"
          >
            <span v-if="!showAll">See all</span>
            <span v-else>See less</span>
          </a>
        </li>
      </ul>
    </li>
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
