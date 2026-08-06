<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, useId, watch } from 'vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import Pagination from '@/components/Pagination.vue'
import { ROUTES } from '@/router/router'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useTemplateStateFilterStore } from '@/stores/state-filter-store'
import { contractTemplateStates } from '@/types/contract-template-state'
import { compareValues } from '@/utils/comparison'
import TemplateListItem from './TemplateListItem.vue'
import TemplateListSearch from './TemplateListSearch.vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

const templatesStore = useContractTemplatesStore()
const { loading, error } = storeToRefs(templatesStore)

const templates: Ref<PartialContractTemplate[]> = ref([])

const pageLimits = ref([25, 50, 100])
const limit = ref(pageLimits.value[0] ?? 25)
const currentPage = ref(1)
const hasNextPage = ref(true)

const pageLimiterId = useId()

const searchResults: Ref<PartialContractTemplate[] | null> = ref(null)

watch([currentPage, limit, searchResults], async () => {
  if (searchResults.value !== null) {
    const offset = (currentPage.value - 1) * limit.value
    const end = offset + limit.value
    templates.value = searchResults.value.slice(offset, end)
    hasNextPage.value = searchResults.value.length > end
  } else {
    await setPaginatedTemplates()
  }
})

onMounted(async () => {
  await setPaginatedTemplates()
})

const setPaginatedTemplates = async () => {
  await templatesStore.loadPaginatedTemplates(currentPage.value, limit.value)
  templates.value = templatesStore.paginatedTemplates
  hasNextPage.value = templates.value.length === limit.value
}

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

const sortedTemplates = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return templates.value
  }
  return templates.value
    .slice()
    .sort((templateA, templateB) => compareValues(templateA, templateB, sortBy.value, sortOrder.value))
})

const hasTemplates = computed(() => templates.value.length > 0)

const { isCreator, isManager } = useTemplatePermissions()

const isRepositoryEmpty = computed(
  () => !hasTemplates.value && searchResults.value === null && !stateFilterStore.hasFilters && currentPage.value === 1,
)

const filteredTemplates = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedTemplates.value.filter((template) => stateFilterStore.hasFilter(template.state))
  }
  return sortedTemplates.value
})

const applySearchResult = (searchResult: PartialContractTemplate[] | null) => {
  searchResults.value = searchResult
  currentPage.value = 1
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="loading" class="pl-4" role="status" aria-live="polite">Loading templates...</div>
    <div v-else-if="error" class="pl-4" role="status" aria-live="assertive">{{ error }}</div>
    <ul v-else class="list flex-1 overflow-y-auto">
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
        :key="`${template.did}|${template.version}`"
        :template="template"
      />
      <li v-if="isRepositoryEmpty" class="flex flex-col items-start gap-2 px-4 py-8">
        <p class="font-semibold">No templates yet.</p>
        <p class="max-w-prose text-sm text-base-content/70">
          Templates are the reusable blueprints contracts are created from. It must pass review, approval and catalogue
          registration before contracts can use it.
        </p>
        <RouterLink
          v-if="isCreator || isManager"
          :to="{ name: ROUTES.TEMPLATES.NEW }"
          class="btn mt-1 btn-sm btn-primary"
        >
          New Template
        </RouterLink>
      </li>
      <li v-else-if="filteredTemplates.length < 1" class="px-4">No templates found</li>
    </ul>
    <div
      class="mt-2 flex w-full shrink-0 flex-nowrap items-center gap-3 border-t border-base-content/10 bg-base-100 px-4 py-4"
    >
      <label :for="pageLimiterId" class="sr-only">Page limit</label>
      <select :id="pageLimiterId" v-model.number="limit" class="select max-w-30 select-sm" @change="currentPage = 1">
        <option disabled>Pick a page limit</option>
        <option v-for="pageLimit in pageLimits" :key="pageLimit">{{ pageLimit }}</option>
      </select>
      <div class="flex flex-1 justify-center">
        <Pagination
          :current-page="currentPage"
          :has-next-page="hasNextPage"
          @next-page="currentPage++"
          @previous-page="currentPage--"
        />
      </div>
    </div>
  </div>
</template>
