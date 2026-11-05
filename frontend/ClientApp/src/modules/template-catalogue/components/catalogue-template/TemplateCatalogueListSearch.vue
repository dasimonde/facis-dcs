<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import ListSearch from '@/components/lists/ListSearch.vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import { type PartyType, partyTypes } from '@/types/party-type'
import type { TemplateCatalogueSearchRequest } from '@/models/requests/template-catalogue-integration-request'
import type { TemplateCatalogueRetrieveResponse } from '@/models/responses/template-catalogue-integration-response'
import type { TemplateResourcesItem } from '@template-catalogue/models/template-resource'

defineProps<{
  templates: TemplateResourcesItem[]
}>()

const partyType = ref<PartyType | ''>('')
const partyTypeId = useId()
const extraRequest = computed(() => ({ party_type: partyType.value || undefined }))
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
  const params: TemplateCatalogueSearchRequest = {
    party_type: partyTypes.find((option) => option.value === request.party_type)?.value,
    offset: 0,
    limit: 0,
  }
  if (typeof request.did === 'string' && request.did) {
    params.did = request.did
  }
  const version = Number(request.version)
  if (!Number.isNaN(version) && version > 0) {
    params.version = version
  }
  if (typeof request.name === 'string' && request.name) {
    params.name = request.name
  }
  if (typeof request.description === 'string' && request.description) {
    params.description = request.description
  }
  return responseMapper(await templateCatalogueIntegrationService.search_template(params))
}
</script>

<template>
  <div class="flex w-full min-w-0 flex-col gap-3">
    <ListSearch
      class="w-full min-w-0"
      :items="templates"
      :filter-labels="filterLabels"
      :extra-request="extraRequest"
      :empty-item="emptyTemplate"
      :search-fn="searchFn"
      placeholder="Search catalogue templates"
      @search-result="(result) => emit('searchResult', result)"
    />

    <section class="flex flex-col gap-2 border-t border-base-300 pt-3" aria-label="Refine catalogue templates">
      <div class="flex min-w-36 flex-col gap-1">
        <label :for="partyTypeId" class="text-xs font-medium text-base-content/70">Party type</label>
        <select :id="partyTypeId" v-model="partyType" class="select-bordered select w-max min-w-44">
          <option value="">All party types</option>
          <option v-for="option in partyTypes" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </div>
    </section>
  </div>
</template>
