<script setup lang="ts">
import ListSearch from '@/components/lists/ListSearch.vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateCatalogueRetrieveResponse } from '@/models/responses/template-catalogue-integration-response'
import type { TemplateResourcesItem } from '@template-catalogue/models/template-resource'

defineProps<{
  templates: TemplateResourcesItem[]
}>()

const emit = defineEmits<{
  searchResult: [value: TemplateResourcesItem[] | null]
}>()

const filterLabels: Partial<Record<keyof TemplateResourcesItem, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  version: 'Version',
}

const emptyTemplate: TemplateResourcesItem = {
  did: '',
  version: 1,
  name: '',
  description: '',
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
