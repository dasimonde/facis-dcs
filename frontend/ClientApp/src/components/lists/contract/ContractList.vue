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
  items: Contract[]
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

const searchedItems: Ref<Contract[]> = ref(props.items)

const sortedItems = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return searchedItems.value
  }
  return searchedItems.value
    .slice()
    .sort((contractA, contractB) => compareValues(contractA, contractB, sortBy.value, sortOrder.value))
})

const filteredItems = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedItems.value.filter((item) => stateFilterStore.hasFilter(item.state))
  }
  return sortedItems.value
})

const applySearchResult = (searchResult: Contract[]) => {
  searchedItems.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="tracking-wide px-4 flex justify-between flex-col sm:flex-row">
      <ListStateFilter label="Contract" :filters="contractStates" store-type="contracts" />
      <ContractListSearch :items="items" class="flex-1" @search-result="applySearchResult" />
      <ListSort
        :class="{ 'btn-disabled': sortedItems.length === 0 }"
        :sorter="sorter"
        v-model:sort-by="sortBy"
        v-model:sort-order="sortOrder"
      />
    </li>
    <template v-if="filteredItems.length > 0">
      <ContractListItem v-for="item in filteredItems" :key="`${item.did}|${item.contract_version}`" :item="item" />
    </template>
    <li v-else class="px-4">No contracts found.</li>
  </ul>
</template>
