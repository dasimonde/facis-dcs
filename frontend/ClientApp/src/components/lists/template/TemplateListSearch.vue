<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import type { ContractTemplateSearchResponse } from '@/models/responses/template-response'
import { contractTemplateService } from '@/services/contract-template-service'
import ListSearch from '../ListSearch.vue'

defineProps<{
  templates: PartialContractTemplate[]
}>()

const emit = defineEmits<{
  searchResult: [value: PartialContractTemplate[]]
}>()

const filterLabels: Partial<Record<keyof PartialContractTemplate, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  document_number: 'Document number',
  version: 'Version',
  template_data: 'Template Data',
}

const emptyTemplate: PartialContractTemplate = {
  did: '',
  document_number: '',
  version: 1,
  created_at: '',
  updated_at: '',
  name: '',
  template_type: 'FRAME_CONTRACT',
  state: 'DRAFT',
  created_by: '',
}

const responseMapper = (response: ContractTemplateSearchResponse) =>
  response.map((item) => ({
    did: item.did,
    name: item.name,
    description: item.description,
    version: parseInt(item.version, 10),
    state: item.state,
    updated_at: item.updated_at,
    created_at: item.created_at,
    document_number: item.document_number,
    template_type: item.template_type,
    created_by: item.created_by,
  }))
</script>

<template>
  <ListSearch
    :items="templates"
    :filter-labels="filterLabels"
    :empty-item="emptyTemplate"
    :search-fn="async (request) => responseMapper(await contractTemplateService.search(request))"
    placeholder="Search templates"
    @search-result="(result) => emit('searchResult', result)"
  />
</template>
