<script setup lang="ts">
import type { ContractNegotiationTask } from '@/models/contract/contract-negotiation-task'
import { ROUTES } from '@/router/router'
import { useContractsStore } from '@/stores/contracts-store'
import { useNegotiationTaskStateFilterStore } from '@/stores/state-filter-store'
import { ContractState } from '@/types/contract-state'
import { negotiationTaskStates } from '@/types/negotiation-task-state'
import { compareValues } from '@/utils/comparison'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import TaskListSearch from './TaskListSearch.vue'

const props = defineProps<{
  tasks: ContractNegotiationTask[]
}>()

const contractsStore = useContractsStore()
const stateFilterStore = useNegotiationTaskStateFilterStore()

const sorter = new Map<keyof ContractNegotiationTask, string>([
  ['created_at', 'Creation date'],
  ['state', 'Task state'],
])
const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const searchedTasks: Ref<ContractNegotiationTask[]> = ref([])
const isSearchActive = ref(false)

const displayedTasks = computed(() => {
  return isSearchActive.value ? searchedTasks.value : props.tasks
})

const sortedTasks = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return displayedTasks.value
  }
  return displayedTasks.value.slice().sort((taskA, taskB) => compareValues(taskA, taskB, sortBy.value, sortOrder.value))
})

const hasTasks = computed(() => props.tasks.length > 0)

const filteredTasks = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedTasks.value.filter((task) => stateFilterStore.hasFilter(task.state))
  }
  return sortedTasks.value
})

const getContractName = (task: ContractNegotiationTask) => {
  return contractsStore.findContractByDid(task.did)?.name ?? 'Nameless Contract'
}

const applySearchResult = (searchResult: ContractNegotiationTask[]) => {
  isSearchActive.value = searchResult.length !== props.tasks.length
  searchedTasks.value = searchResult
}

const resolveViewRouteName = (task: ContractNegotiationTask) => {
  const currentState = contractsStore.findContractByDid(task.did)?.state
  if (currentState === ContractState.negotiation) {
    return ROUTES.CONTRACTS.NEGOTIATE
  }
  return ROUTES.CONTRACTS.VIEW
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="flex w-full flex-col justify-end px-4 tracking-wide sm:flex-row">
      <TaskListSearch class="flex-1" :tasks="tasks" placeholder="Search contracts" @search-result="applySearchResult" />
      <ListStateFilter
        label="Negotiation Task"
        :filters="negotiationTaskStates"
        store-type="negotiationTasks"
        :disabled="!hasTasks"
      />
      <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" :disabled="!hasTasks" />
    </li>
    <template v-if="filteredTasks.length > 0">
      <li v-for="task in filteredTasks" :key="task.did" class="list-row">
        <div class="list-col-grow card border-base-content/10 bg-base-100 card-border hover:bg-base-300">
          <div class="card-body">
            <h2 class="card-title flex-wrap justify-between">
              <div>Negotiation Task for Contract: {{ getContractName(task) }}</div>
              <div class="flex-1"></div>
              <div class="badge badge-secondary">{{ task.state }}</div>
            </h2>
            <div class="flex justify-between">
              <div v-if="task.contract_version">Version: {{ task.contract_version }}</div>
            </div>
            <div class="flex justify-between">
              <div>Creation date: {{ new Date(task.created_at).toLocaleDateString() }}</div>
              <div class="card-actions justify-end">
                <RouterLink
                  :to="{ name: resolveViewRouteName(task), params: { did: task.did } }"
                  class="btn btn-sm btn-primary"
                >
                  View
                </RouterLink>
              </div>
            </div>
          </div>
        </div>
      </li>
    </template>
    <li v-else class="px-4">No negotiation tasks found.</li>
  </ul>
</template>
