<script setup lang="ts">
import type { ContractNegotiationTask } from '@/models/contract/contract-negotiation-task'
import { ROUTES } from '@/router/router'
import { useContractsStore } from '@/stores/contracts-store'
import { useNegotiationTaskStateFilterStore } from '@/stores/state-filter-store'
import { negotiationTaskStates } from '@/types/negotiation-task-state'
import { compareValues } from '@/utils/comparison'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import TaskListSearch from './TaskListSearch.vue'

const props = defineProps<{
  items: ContractNegotiationTask[]
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

const searchedItems: Ref<ContractNegotiationTask[]> = ref([])
const isSearchActive = ref(false)

const displayedItems = computed(() => {
  return isSearchActive.value ? searchedItems.value : props.items
})

const sortedItems = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return displayedItems.value
  }
  return displayedItems.value.slice().sort((taskA, taskB) => compareValues(taskA, taskB, sortBy.value, sortOrder.value))
})

const filteredItems = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedItems.value.filter((item) => stateFilterStore.hasFilter(item.state))
  }
  return sortedItems.value
})

const getContractName = (item: ContractNegotiationTask) => {
  return contractsStore.contracts.find((contract) => contract.did === item.did)?.name ?? 'Nameless Contract'
}

const applySearchResult = (searchResult: ContractNegotiationTask[]) => {
  isSearchActive.value = searchResult.length !== props.items.length
  searchedItems.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="tracking-wide w-full px-4 flex justify-end flex-col sm:flex-row">
      <ListStateFilter label="Negotiation Task" :filters="negotiationTaskStates" store-type="negotiationTasks" />
      <TaskListSearch class="flex-1" :items="items" placeholder="Search contracts" @search-result="applySearchResult" />
      <ListSort :sorter="sorter" v-model:sort-by="sortBy" v-model:sort-order="sortOrder" />
    </li>
    <template v-if="filteredItems.length > 0">
      <li v-for="item in filteredItems" :key="item.did" class="list-row">
        <div class="list-col-grow card bg-base-200 card-border hover:bg-base-300">
          <div class="card-body">
            <h2 class="card-title flex-wrap justify-between">
              <div>Negotiation Task for Contract: {{ getContractName(item) }}</div>
              <div class="flex-1"></div>
              <div class="badge badge-secondary">{{ item.state }}</div>
            </h2>
            <div class="flex justify-between">
              <div v-if="item.contract_version">Version: {{ item.contract_version }}</div>
            </div>
            <div class="flex justify-between">
              <div>Creation date: {{ new Date(item.created_at).toLocaleDateString() }}</div>
              <div class="card-actions justify-end">
                <RouterLink
                  :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: item.did } }"
                  class="btn btn-sm btn-primary rounded-box"
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
