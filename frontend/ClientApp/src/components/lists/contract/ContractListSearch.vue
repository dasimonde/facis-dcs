<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import ListSearch from '../ListSearch.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import type { ContractSearchResponse } from '@/models/responses/contract-response'

defineProps<{
  items: Contract[]
}>()

const emit = defineEmits<{
  searchResult: [value: Contract[]]
}>()

const filterLabels: Partial<Record<keyof Contract, string>> = {
  name: 'Name',
  description: 'Description',
  contract_version: 'Version',
}

const responseMapper = (response: ContractSearchResponse) =>
  response.map(
    (item) =>
      ({
        did: item.did,
        name: item.name,
        description: item.description,
        contract_version: item.contract_version,
        state: item.state,
        updated_at: item.updated_at,
        created_at: item.created_at,
      }) as Contract,
  )
const empty: Contract = { did: '', created_at: '', state: 'DRAFT', updated_at: '' }
</script>
<template>
  <ListSearch
    :items="items"
    :filter-labels="filterLabels"
    :search-fn="async (request) => responseMapper(await contractWorkflowService.search(request))"
    :empty-item="empty"
    placeholder="Search contracts"
    @search-result="(result) => emit('searchResult', result)"
  />
</template>
