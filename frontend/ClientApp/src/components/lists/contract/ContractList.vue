<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, useId, watch } from 'vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import Pagination from '@/components/Pagination.vue'
import { ROUTES } from '@/router/router'
import { useContractsStore } from '@/stores/contracts-store'
import { useContractStateFilterStore } from '@/stores/state-filter-store'
import { contractStates } from '@/types/contract-state'
import { compareValues } from '@/utils/comparison'
import ContractListItem from './ContractListItem.vue'
import ContractListSearch from './ContractListSearch.vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import type { Contract } from '@/models/contract/contract'

const contractsStore = useContractsStore()
const { loading, error } = storeToRefs(contractsStore)

const contracts: Ref<Contract[]> = ref([])

const pageLimits = ref([25, 50, 100])
const limit = ref(pageLimits.value[0] ?? 25)
const currentPage = ref(1)
const hasNextPage = ref(true)

const pageLimiterId = useId()

const searchResults: Ref<Contract[] | null> = ref(null)

watch([currentPage, limit, searchResults], async () => {
  if (searchResults.value !== null) {
    const offset = (currentPage.value - 1) * limit.value
    const end = offset + limit.value
    contracts.value = searchResults.value.slice(offset, end)
    hasNextPage.value = searchResults.value.length > end
  } else {
    await setPaginatedContracts()
  }
})

onMounted(async () => {
  await setPaginatedContracts()
})

const setPaginatedContracts = async () => {
  await contractsStore.loadPaginatedContracts(currentPage.value, limit.value)
  contracts.value = contractsStore.paginatedContracts
  hasNextPage.value = contracts.value.length === limit.value
}

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

const sortedContracts = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return contracts.value
  }
  return contracts.value
    .slice()
    .sort((contractA, contractB) => compareValues(contractA, contractB, sortBy.value, sortOrder.value))
})

const hasContracts = computed(() => contracts.value.length > 0)

const { isCreator } = useContractPermissions()

const isRepositoryEmpty = computed(
  () => !hasContracts.value && searchResults.value === null && !stateFilterStore.hasFilters && currentPage.value === 1,
)

const filteredContracts = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedContracts.value.filter((contract) => stateFilterStore.hasFilter(contract.state))
  }
  return sortedContracts.value
})

const applySearchResult = (searchResult: Contract[] | null) => {
  searchResults.value = searchResult
  currentPage.value = 1
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="loading" class="pl-4" role="status" aria-live="polite">Loading contracts...</div>
    <div v-else-if="error" class="pl-4" role="status" aria-live="assertive">{{ error }}</div>
    <ul v-else class="list flex-1 overflow-y-auto">
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
      <li v-else-if="isRepositoryEmpty" class="flex flex-col items-start gap-2 px-4 py-8">
        <p class="font-semibold">No contracts yet.</p>
        <p class="max-w-prose text-sm text-base-content/70">
          Contracts are created from templates that are approved and registered in the Template Catalogue, then move
          through negotiation, review, approval and signing. Start by creating one from a registered template.
        </p>
        <RouterLink v-if="isCreator" :to="{ name: ROUTES.CONTRACTS.NEW }" class="btn mt-1 btn-sm btn-primary">
          New Contract
        </RouterLink>
      </li>
      <li v-else class="px-4">No contracts found.</li>
    </ul>
    <div
      class="mt-2 flex w-full shrink-0 flex-nowrap items-center gap-3 border-t border-base-content/10 bg-base-100 px-4 py-4"
    >
      <label :for="pageLimiterId" class="sr-only">Page limit</label>
      <select :id="pageLimiterId" v-model.number="limit" class="select max-w-30 select-sm" @change="currentPage = 1">
        <option disabled>Pick a page limit</option>
        <option v-for="pageLimit in pageLimits" :key="pageLimit">{{ pageLimit }}</option>
      </select>
      <div class="flex flex-1 justify-center">
        <Pagination
          :current-page="currentPage"
          :has-next-page="hasNextPage"
          @next-page="currentPage++"
          @previous-page="currentPage--"
        />
      </div>
    </div>
  </div>
</template>
