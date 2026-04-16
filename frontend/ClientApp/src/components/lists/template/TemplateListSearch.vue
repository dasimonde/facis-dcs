<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import type { ContractTemplateSearchResponse } from '@/models/responses/template-response'
import { contractTemplateService } from '@/services/contract-template-service'
import ListSearch from '../ListSearch.vue'

const props = defineProps<{
  items: PartialContractTemplate[]
}>()

const emit = defineEmits<{
  searchResult: [value: PartialContractTemplate[]]
}>()

const filterLabels: Partial<Record<keyof PartialContractTemplate, string>> = {
  name: 'Name',
  description: 'Description',
  document_number: 'Document number',
  version: 'Version',
  // filter: 'Filter',
}

const emptyTemplate: PartialContractTemplate = {
  did: '',
  document_number: '',
  version: -1,
  created_at: '',
  updated_at: '',
  name: '',
  template_type: 'FRAME_CONTRACT',
  state: 'DRAFT',
  created_by: '',
}

const responseMapper = (response: ContractTemplateSearchResponse) =>
  response.map(
    (item) =>
      ({
        did: item.did,
        name: item.name,
        description: item.description,
        version: item.version,
        state: item.state,
        updated_at: item.updated_at,
        created_at: item.created_at,
        document_number: item.document_number,
        template_type: item.template_type,
      }) as PartialContractTemplate,
  )
</script>

<template>
  <ListSearch
    :items="items"
    :filter-labels="filterLabels"
    :empty-item="emptyTemplate"
    :search-fn="async (request) => responseMapper(await contractTemplateService.search(request))"
    placeholder="Search templates"
    @search-result="(result) => emit('searchResult', result)"
  />
</template>
