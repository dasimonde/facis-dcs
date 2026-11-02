<script setup lang="ts">
import { computed, ref } from 'vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import ListSearch from '../ListSearch.vue'
import type { Contract } from '@/models/contract/contract'
import type { ContractSearchResponse } from '@/models/responses/contract-response'

defineProps<{
  contracts: Contract[]
}>()

const emit = defineEmits<{
  searchResult: [value: Contract[] | null]
}>()

const filterLabels: Partial<Record<keyof Contract | 'party' | 'jurisdiction', string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  contract_version: 'Version',
  contract_data: 'Contract Data',
  party: 'Party',
  template_did: 'Template',
  jurisdiction: 'Jurisdiction',
}

const deadlineUntil = ref('')

const extraRequest = computed(() => {
  if (!deadlineUntil.value) return {}
  return { valid_until: `${deadlineUntil.value}T23:59:59Z` }
})

const responseMapper = (response: ContractSearchResponse): Contract[] =>
  response.map((item) => ({
    did: item.did,
    name: item.name,
    description: item.description,
    contract_version: item.contract_version,
    state: item.state,
    updated_at: item.updated_at,
    created_at: item.created_at,
    start_date: item.start_date,
    exp_date: item.exp_date,
    exp_policy: item.exp_policy,
    exp_notice_period: item.exp_notice_period,
    responsible: item.responsible,
    created_by: '',
  }))

const empty: Contract = { did: '', created_at: '', state: 'DRAFT', updated_at: '', created_by: '', contract_version: 1 }

let lastRequest: Record<string, unknown> | null = null

async function search(request: Record<string, unknown>): Promise<Contract[]> {
  lastRequest = request
  return responseMapper(await contractWorkflowService.search(request))
}

// Runs the last search again, so a reload keeps the user's search.
async function rerun(): Promise<Contract[] | null> {
  return lastRequest ? search(lastRequest) : null
}

defineExpose({ rerun })
</script>

<template>
  <ListSearch
    :items="contracts"
    :filter-labels="filterLabels"
    :search-fn="search"
    :empty-item="empty"
    :extra-request="extraRequest"
    placeholder="Search contracts"
    class="min-w-0 flex-1"
    @search-result="(result) => emit('searchResult', result)"
  >
    <template #before-search-button="{ submit }">
      <label
        class="deadline-until-field input join-item -mt-px w-full shrink-0 rounded-none border-s-0 input-secondary sm:mt-0 sm:w-auto"
      >
        <span class="label whitespace-nowrap">Deadline until</span>
        <input
          v-model="deadlineUntil"
          type="date"
          class="deadline-until-date grow"
          aria-label="Deadline until"
          data-testid="contract-deadline-filter"
          @keydown.enter.prevent="submit()"
        />
      </label>
    </template>
  </ListSearch>
</template>

<style scoped>
.deadline-until-field :deep(.label) {
  border-inline-end: none;
  margin-inline-end: 0.25rem;
}

.deadline-until-field :deep(input.deadline-until-date) {
  position: relative;
  padding-inline-start: 1.35em;
}

.deadline-until-field :deep(input.deadline-until-date::-webkit-calendar-picker-indicator) {
  cursor: pointer;
  position: absolute;
  inset-inline-start: 0;
  inset-inline-end: auto;
  width: 1em;
  height: 1em;
  margin: 0;
}
</style>
