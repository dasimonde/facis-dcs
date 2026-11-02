<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, useId, useTemplateRef, watch } from 'vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import Pagination from '@/components/Pagination.vue'
import { type ContractChange, useContractChanges } from '@/composables/useContractChanges'
import { ROUTES } from '@/router/router'
import { archiveService } from '@/services/archive-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useContractStateFilterStore } from '@/stores/state-filter-store'
import { contractStates } from '@/types/contract-state'
import { reportBulk, runBulk } from '@/utils/bulk-action'
import { compareValues } from '@/utils/comparison'
import ContractListItem from './ContractListItem.vue'
import ContractListSearch from './ContractListSearch.vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import type { Contract } from '@/models/contract/contract'
import type { ContractSortKey } from '@/models/requests/contract-request'
import type { ContractSearchResponse } from '@/models/responses/contract-response'

const contractsStore = useContractsStore()
const { loading, error } = storeToRefs(contractsStore)

// The shown page: filtered and sorted before the page is cut (DCS-IR-CWE-11).
const contracts: Ref<Contract[]> = ref([])

const pageLimits = ref([25, 50, 100])
const limit = ref(pageLimits.value[0] ?? 25)
const currentPage = ref(1)
const hasNextPage = ref(true)

const pageLimiterId = useId()

const searchResults: Ref<Contract[] | null> = ref(null)
const listSearch = useTemplateRef('list-search')

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
const stateFilters = computed(() => [...stateFilterStore.stateFilters])

// compareValues sorts dates newest first and text A to Z for order 1; the
// server receives the same direction.
function serverSortOrder(): 'asc' | 'desc' {
  const isDate = sortBy.value === 'created_at' || sortBy.value === 'updated_at'
  return (sortOrder.value === 1) === isDate ? 'desc' : 'asc'
}

// A search result is complete on the client and is filtered, sorted and cut
// there; otherwise the server filters, sorts and cuts the page.
async function showPage() {
  if (searchResults.value !== null) {
    const matching = searchResults.value
      .filter((contract) => !stateFilterStore.hasFilters || stateFilterStore.hasFilter(contract.state))
      .sort((contractA, contractB) => compareValues(contractA, contractB, sortBy.value, sortOrder.value))
    const offset = (currentPage.value - 1) * limit.value
    contracts.value = matching.slice(offset, offset + limit.value)
    hasNextPage.value = matching.length > offset + limit.value
    return
  }
  await contractsStore.loadPaginatedContracts(currentPage.value, limit.value, {
    ...(stateFilterStore.hasFilters ? { state: stateFilters.value } : {}),
    sort_by: sortBy.value as ContractSortKey,
    sort_order: serverSortOrder(),
  })
  contracts.value = contractsStore.paginatedContracts
  hasNextPage.value = contracts.value.length === limit.value
}

const selectedDids = ref(new Set<string>())

// A new filter, sort, page size or search starts at page 1 and clears the
// selection, so a bulk action never covers a row that is not shown.
watch([limit, searchResults, stateFilters, sortBy, sortOrder], () => {
  selectedDids.value = new Set()
  if (currentPage.value === 1) void showPage()
  else currentPage.value = 1
})
watch(currentPage, () => void showPage())

onMounted(showPage)

const hasContracts = computed(() => contracts.value.length > 0)

const { isCreator } = useContractPermissions()

const isRepositoryEmpty = computed(
  () => !hasContracts.value && searchResults.value === null && !stateFilterStore.hasFilters && currentPage.value === 1,
)

const applySearchResult = (searchResult: Contract[] | null) => {
  searchResults.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())

// Live updates (DCS-FR-CWE-24): a changed contract on this page is fetched
// again on its own; a change to any other contract offers a reload, so a
// change never reloads the whole list unasked.
const listOutdated = ref(false)

async function applyChange(change: ContractChange) {
  const index = contracts.value.findIndex((contract) => contract.did === change.did)
  if (index < 0) {
    listOutdated.value = true
    return
  }
  let fresh: ContractSearchResponse[number] | undefined
  try {
    ;[fresh] = await contractWorkflowService.search({ did: change.did })
  } catch {
    listOutdated.value = true
    return
  }
  const current = contracts.value.findIndex((contract) => contract.did === change.did)
  if (current < 0) return
  if (fresh) contracts.value.splice(current, 1, { ...contracts.value[current]!, ...fresh })
  else contracts.value.splice(current, 1)
}
const emit = defineEmits<{ change: [change: ContractChange] }>()
useContractChanges(
  (change) => {
    emit('change', change)
    void applyChange(change)
  },
  () => {
    listOutdated.value = true
  },
)

// Loads the list once and keeps an active search.
async function reloadList() {
  listOutdated.value = false
  if (searchResults.value !== null) {
    searchResults.value = (await listSearch.value?.rerun()) ?? null
  } else {
    await showPage()
  }
}

// Bulk actions (DCS-FR-CWE-24): each one calls the per-contract endpoint for
// every selected contract and reports one summary.
const authStore = useAuthStore()
const { isApprover, isManager } = useContractPermissions()
const isArchiveManager = computed(() => authStore.user?.roles?.includes('ARCHIVE_MANAGER') ?? false)
// Rows are selectable only for a user who holds a bulk action.
const canBulk = computed(() => isManager.value || isApprover.value || isArchiveManager.value)
const selectedContracts = computed(() => contracts.value.filter((contract) => selectedDids.value.has(contract.did)))
const bulkRunning = ref(false)
const bulkModal = useTemplateRef('bulk-modal')

function setSelected(did: string, value: boolean | undefined) {
  const next = new Set(selectedDids.value)
  if (value) next.add(did)
  else next.delete(did)
  selectedDids.value = next
}

async function bulk(
  label: string,
  message: string,
  action: (contract: Contract, reason?: string) => Promise<unknown>,
  needsReason = false,
) {
  const dialog = bulkModal.value
  if (!dialog || selectedContracts.value.length === 0) return
  const { isCanceled, data } = await dialog.reveal({
    message: `${message} (${selectedContracts.value.length} selected)`,
    ...(needsReason ? { editor: { requiredText: true, placeholder: 'Reason' } } : {}),
  })
  if (isCanceled || (needsReason && !data)) return
  bulkRunning.value = true
  try {
    reportBulk(label, await runBulk(selectedContracts.value, (contract) => action(contract, data)))
    selectedDids.value = new Set()
    await reloadList()
  } finally {
    bulkRunning.value = false
  }
}

const bulkRenew = () =>
  bulk('Renew', 'Create a renewal contract from each selected contract?', (contract) =>
    contractWorkflowService.renew({ did: contract.did, updated_at: contract.updated_at }),
  )
const bulkTerminate = () =>
  bulk(
    'Terminate',
    'Terminate the selected contracts? The reason is recorded with each termination.',
    (contract, reason) =>
      contractWorkflowService.terminate({ did: contract.did, updated_at: contract.updated_at, reason: reason ?? '' }),
    true,
  )
const bulkPublish = () =>
  bulk(
    'Register in catalogue',
    'Register the selected contracts in the Federated Catalogue? Each entry shows name, state, version, template and validity period, without content or parties.',
    (contract) => contractWorkflowService.publishCatalogue(contract.did),
  )
const bulkArchive = () =>
  bulk('Archive', 'Archive the current version of each selected contract?', (contract) =>
    archiveService.storeContract(contract.did),
  )
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- The search, filter and sort controls stay mounted while a filter or sort reloads the page. -->
    <ul class="list flex-1 overflow-y-auto">
      <li class="flex flex-wrap items-center gap-3 px-4 py-2 tracking-wide [&>.join]:m-0">
        <ContractListSearch
          ref="list-search"
          :contracts="contracts"
          class="w-full min-w-0 lg:w-auto lg:flex-1"
          @search-result="applySearchResult"
        />
        <div class="ml-auto flex shrink-0 items-center gap-3 [&>button]:m-0">
          <ListStateFilter
            label="Contract"
            :filters="contractStates"
            store-type="contracts"
            :disabled="isRepositoryEmpty"
          />
          <ListSort
            v-model:sort-by="sortBy"
            v-model:sort-order="sortOrder"
            :sorter="sorter"
            :disabled="isRepositoryEmpty"
          />
        </div>
      </li>
      <li v-if="loading" class="pl-4"><span role="status" aria-live="polite">Loading contracts...</span></li>
      <li v-else-if="error" class="pl-4">
        <span role="status" aria-live="assertive">{{ error }}</span>
      </li>
      <template v-else>
        <li v-if="listOutdated" class="px-4" data-testid="contracts-outdated">
          <div role="status" class="alert alert-info">
            Contracts changed since this list was loaded.
            <button class="btn btn-sm" @click="reloadList">Reload</button>
          </div>
        </li>
        <li
          v-if="selectedContracts.length > 0"
          class="flex flex-wrap items-center gap-2 px-4"
          data-testid="contracts-bulk-actions"
        >
          <span class="text-sm">{{ selectedContracts.length }} selected</span>
          <button v-if="isManager" class="btn btn-sm" :disabled="bulkRunning" @click="bulkRenew">Renew selected</button>
          <button v-if="isManager" class="btn btn-sm btn-error" :disabled="bulkRunning" @click="bulkTerminate">
            Terminate selected
          </button>
          <button v-if="isApprover || isManager" class="btn btn-sm" :disabled="bulkRunning" @click="bulkPublish">
            Register selected in catalogue
          </button>
          <button v-if="isArchiveManager" class="btn btn-sm" :disabled="bulkRunning" @click="bulkArchive">
            Archive selected
          </button>
          <button class="btn btn-ghost btn-sm" :disabled="bulkRunning" @click="selectedDids = new Set()">
            Clear selection
          </button>
        </li>
        <template v-if="contracts.length > 0">
          <ContractListItem
            v-for="contract in contracts"
            :key="`${contract.did}|${contract.contract_version}`"
            :contract="contract"
            :selected="canBulk ? selectedDids.has(contract.did) : undefined"
            @update:selected="setSelected(contract.did, $event)"
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
      </template>
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
    <ConfirmationModal ref="bulk-modal" />
  </div>
</template>
