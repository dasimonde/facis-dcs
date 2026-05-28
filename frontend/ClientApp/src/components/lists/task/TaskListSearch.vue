<script setup lang="ts" generic="T extends { did: string; type: 'template' | 'contract' }">
import type { PartialContractTemplate } from '@/models/contract-template'
import type { Contract } from '@/models/contract/contract'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import { computed } from 'vue'
import ListSearch from '../ListSearch.vue'

type Searchable = PartialContractTemplate | Contract

const props = defineProps<{
  tasks: T[]
  placeholder?: string
}>()

const emit = defineEmits<{
  searchResult: [value: T[]]
}>()

const templatesStore = useContractTemplatesStore()
const contractsStore = useContractsStore()

const filterLabels: Partial<Record<keyof Searchable, string>> = {
  name: 'Name',
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

const searchableItems = computed(() => {
  const items: Searchable[] = []
  const seenDids = new Set<string>()

  for (const task of props.tasks) {
    if (seenDids.has(task.did)) continue
    seenDids.add(task.did)

    if (task.type === 'template') {
      const template = templatesStore.findTemplateByDid(task.did)
      if (template) {
        items.push(template)
      }
    } else {
      const contract = contractsStore.findContractByDid(task.did)
      if (contract) {
        items.push(contract)
      }
    }
  }
  return items
})

const search = (request: Record<string, unknown>): Promise<Searchable[]> => {
  if (!request.name) return Promise.resolve(searchableItems.value)

  const query = (typeof request.name === 'string' ? request.name : '').toLowerCase()
  return Promise.resolve(
    searchableItems.value.filter((item) => {
      const name = item.name ? String(item.name).toLowerCase() : ''
      return name.includes(query)
    }),
  )
}

const handleSearchResult = (searchResults: Searchable[]) => {
  const resultDids = new Set(searchResults.map((item) => item.did))
  const filteredTasks = props.tasks.filter((task) => resultDids.has(task.did))
  emit('searchResult', filteredTasks)
}
</script>

<template>
  <ListSearch
    :items="searchableItems"
    :filter-labels="filterLabels"
    :search-fn="search"
    :empty-item="emptyTemplate"
    :placeholder="placeholder || 'Search templates/contracts'"
    @search-result="handleSearchResult"
  />
</template>
