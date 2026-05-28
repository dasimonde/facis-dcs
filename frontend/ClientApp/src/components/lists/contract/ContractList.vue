<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { useContractStateFilterStore } from '@/stores/state-filter-store'
import { contractStates } from '@/types/contract-state'
import { compareValues } from '@/utils/comparison'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import ContractListItem from './ContractListItem.vue'
import ContractListSearch from './ContractListSearch.vue'

const props = defineProps<{
  contracts: Contract[]
}>()

const sorter = new Map<keyof Contract, string>([
  ['created_at', 'Creation date'],
  ['updated_at', 'Update date'],
  ['state', 'Contract state'],
  ['name', 'Name'],
])

const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const stateFilterStore = useContractStateFilterStore()

const searchedContracts: Ref<Contract[]> = ref(props.contracts)

const sortedContracts = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return searchedContracts.value
  }
  return searchedContracts.value
    .slice()
    .sort((contractA, contractB) => compareValues(contractA, contractB, sortBy.value, sortOrder.value))
})

const hasContracts = computed(() => props.contracts.length > 0)

const filteredContracts = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedContracts.value.filter((contract) => stateFilterStore.hasFilter(contract.state))
  }
  return sortedContracts.value
})

const applySearchResult = (searchResult: Contract[]) => {
  searchedContracts.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="flex flex-col justify-between px-4 tracking-wide sm:flex-row">
      <ContractListSearch :contracts="contracts" class="flex-1" @search-result="applySearchResult" />
      <ListStateFilter label="Contract" :filters="contractStates" store-type="contracts" :disabled="!hasContracts" />
      <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" :disabled="!hasContracts" />
    </li>
    <template v-if="filteredContracts.length > 0">
      <ContractListItem
        v-for="contract in filteredContracts"
        :key="`${contract.did}|${contract.contract_version}`"
        :contract="contract"
      />
    </template>
    <li v-else class="px-4">No contracts found.</li>
  </ul>
</template>
