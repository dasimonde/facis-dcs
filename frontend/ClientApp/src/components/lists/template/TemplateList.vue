<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import { useTemplateStateFilterStore } from '@/stores/state-filter-store'
import { contractTemplateStates } from '@/types/contract-template-state'
import { compareValues } from '@/utils/comparison'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import TemplateListItem from './TemplateListItem.vue'
import TemplateListSearch from './TemplateListSearch.vue'

const props = defineProps<{
  templates: PartialContractTemplate[]
  hasReviewTask: (template: PartialContractTemplate) => boolean
  hasApprovalTask: (template: PartialContractTemplate) => boolean
}>()

const sorter = new Map<keyof PartialContractTemplate, string>([
  ['created_at', 'Creation date'],
  ['updated_at', 'Update date'],
  ['state', 'Template state'],
  ['name', 'Name'],
])

const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const stateFilterStore = useTemplateStateFilterStore()

const searchedTemplates: Ref<PartialContractTemplate[]> = ref(props.templates)

const sortedTemplates = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return searchedTemplates.value
  }
  return searchedTemplates.value
    .slice()
    .sort((templateA, templateB) => compareValues(templateA, templateB, sortBy.value, sortOrder.value))
})

const hasTemplates = computed(() => props.templates.length > 0)

const filteredTemplates = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedTemplates.value.filter((template) => stateFilterStore.hasFilter(template.state))
  }
  return sortedTemplates.value
})

const applySearchResult = (searchResult: PartialContractTemplate[]) => {
  searchedTemplates.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="flex flex-col justify-between px-4 tracking-wide sm:flex-row">
      <TemplateListSearch :templates="templates" class="flex-1" @search-result="applySearchResult" />
      <ListStateFilter
        label="Template"
        :filters="contractTemplateStates"
        store-type="templates"
        :disabled="!hasTemplates"
      />
      <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" :disabled="!hasTemplates" />
    </li>
    <TemplateListItem
      v-for="template in filteredTemplates"
      :key="`${template.did}|${template.document_number}|${template.version}`"
      :template="template"
      :has-review-task="props.hasReviewTask(template)"
      :has-approval-task="props.hasApprovalTask(template)"
    />
    <li v-if="filteredTemplates.length < 1" class="px-4">No templates found</li>
  </ul>
</template>
