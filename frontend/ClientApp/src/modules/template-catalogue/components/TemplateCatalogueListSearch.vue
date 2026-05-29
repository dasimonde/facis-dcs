<script setup lang="ts">
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import type { TemplateCatalogueRetrieveResponse } from '@/models/responses/template-catalogue-integration-response'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import ListSearch from '@/components/lists/ListSearch.vue'

defineProps<{
  templates: TemplateResourcesItem[]
}>()

const emit = defineEmits<{
  searchResult: [value: TemplateResourcesItem[]]
}>()

const filterLabels: Partial<Record<keyof TemplateResourcesItem, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  document_number: 'Document number',
  version: 'Version',
}

const emptyTemplate: TemplateResourcesItem = {
  did: '',
  document_number: '',
  version: 1,
  name: '',
  description: '',
  template_type: '',
  participant_id: '',
  created_at: '',
  updated_at: '',
}

const responseMapper = (response: TemplateCatalogueRetrieveResponse) => response.items ?? []

const searchFn = async (request: Record<string, unknown>) => {
  const params: Record<string, unknown> = {
    offset: 0,
    limit: 0,
  }
  if (request.did) {
    params.did = request.did
  }
  if (request.document_number) {
    params.document_number = request.document_number
  }
  const version = Number(request.version)
  if (!Number.isNaN(version) && version > 0) {
    params.version = version
  }
  if (request.name) {
    params.name = request.name
  }
  if (request.description) {
    params.description = request.description
  }
  return responseMapper(await templateCatalogueIntegrationService.search_template(params as never))
}
</script>

<template>
  <ListSearch
    :items="templates"
    :filter-labels="filterLabels"
    :empty-item="emptyTemplate"
    :search-fn="searchFn"
    placeholder="Search catalogue templates"
    @search-result="(result) => emit('searchResult', result)"
  />
</template>
