<script setup lang="ts">
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import { compareValues } from '@/utils/comparison'
import { computed, ref, watch } from 'vue'
import ListSort from '@/components/lists/ListSort.vue'
import TemplateCatalogueListItem from './TemplateCatalogueListItem.vue'
import TemplateCatalogueListSearch from './TemplateCatalogueListSearch.vue'

const props = defineProps<{
  templates: TemplateResourcesItem[]
}>()

const sorter = new Map<keyof TemplateResourcesItem, string>([
  ['created_at', 'Creation date'],
  ['name', 'Name'],
])

const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const searchedTemplates = ref<TemplateResourcesItem[]>(props.templates)

const sortedTemplates = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return searchedTemplates.value
  }
  return searchedTemplates.value
    .slice()
    .sort((templateA, templateB) => compareValues(templateA, templateB, sortBy.value, sortOrder.value))
})

const hasTemplates = computed(() => props.templates.length > 0)

const applySearchResult = (searchResult: TemplateResourcesItem[]) => {
  searchedTemplates.value = searchResult
}

watch(
  () => props.templates,
  (value) => {
    searchedTemplates.value = value
  },
)
</script>

<template>
  <ul class="list">
    <li class="flex flex-col justify-between px-4 tracking-wide sm:flex-row">
      <TemplateCatalogueListSearch :templates="templates" class="flex-1" @search-result="applySearchResult" />
      <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" :disabled="!hasTemplates" />
    </li>
    <TemplateCatalogueListItem
      v-for="template in sortedTemplates"
      :key="`${template.did}|${template.document_number}|${template.version}`"
      :template="template"
    />
    <li v-if="sortedTemplates.length < 1" class="px-4">No templates found</li>
  </ul>
</template>
