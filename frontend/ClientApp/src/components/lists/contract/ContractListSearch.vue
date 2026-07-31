<script setup lang="ts">
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

const filterLabels: Partial<Record<keyof Contract, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  contract_version: 'Version',
  contract_data: 'Contract Data',
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
const empty: Contract = { did: '', created_at: '', state: 'DRAFT', updated_at: '', created_by: '', contract_version: 1 }
</script>

<template>
  <ListSearch
    :items="contracts"
    :filter-labels="filterLabels"
    :search-fn="async (request) => responseMapper(await contractWorkflowService.search(request))"
    :empty-item="empty"
    placeholder="Search contracts"
    @search-result="(result) => emit('searchResult', result)"
  />
</template>
