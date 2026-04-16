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
  items: PartialContractTemplate[]
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

const searchedItems: Ref<PartialContractTemplate[]> = ref(props.items)

const sortedItems = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return searchedItems.value
  }
  return searchedItems.value
    .slice()
    .sort((templateA, templateB) => compareValues(templateA, templateB, sortBy.value, sortOrder.value))
})

const filteredItems = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedItems.value.filter((item) => stateFilterStore.hasFilter(item.state))
  }
  return sortedItems.value
})

const applySearchResult = (searchResult: PartialContractTemplate[]) => {
  searchedItems.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="tracking-wide px-4 flex justify-between flex-col sm:flex-row">
      <ListStateFilter label="Template" :filters="contractTemplateStates" store-type="templates" />
      <TemplateListSearch :items="items" class="flex-1" @search-result="applySearchResult" />
      <ListSort :sorter="sorter" v-model:sort-by="sortBy" v-model:sort-order="sortOrder" />
    </li>
    <TemplateListItem
      v-for="item in filteredItems"
      :key="`${item.did}|${item.document_number}|${item.version}`"
      :item="item"
      :has-review-task="props.hasReviewTask(item)"
      :has-approval-task="props.hasApprovalTask(item)"
    />
    <li v-if="filteredItems.length < 1" class="px-4">No templates found</li>
  </ul>
</template>
