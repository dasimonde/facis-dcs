<script setup lang="ts" generic="T extends { did: string; type: 'template' | 'contract' }">
import { computed } from 'vue'
import { contractTemplateService } from '@/services/contract-template-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import ListSearch from '../ListSearch.vue'
import type { Contract } from '@/models/contract/contract'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'
import type { ContractSearchRequest } from '@/models/requests/contract-request'
import type { ContractTemplateSearchRequest } from '@/models/requests/template-request'
import type { ContractSearchResponse } from '@/models/responses/contract-response'
import type { ContractTemplateSearchResponse } from '@/models/responses/template-response'

type Searchable = PartialContractTemplate | Contract

// Filters only the contract search knows (DCS-IR-CWE-07, DCS-FR-CWE-13).
const contractOnlyFilters = ['contract_data', 'party', 'template_did', 'jurisdiction'] as const
type ContractOnlyFilter = (typeof contractOnlyFilters)[number]

const props = defineProps<{
  tasks: T[]
  placeholder?: string
  contractSearchDefaults?: Partial<ContractSearchRequest>
}>()

const emit = defineEmits<{
  searchResult: [value: T[] | null]
}>()

const templatesStore = useContractTemplatesStore()
const contractsStore = useContractsStore()

const filterLabels: Partial<Record<keyof Searchable | ContractOnlyFilter, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  contract_data: 'Contract Data',
  party: 'Party',
  template_did: 'Template',
  jurisdiction: 'Jurisdiction',
}

const emptyItem: Searchable = {
  did: '',
  created_at: '',
  updated_at: '',
  name: '',
  template_type: 'CONTRACT_TEMPLATE',
  party_type: 'SINGLE_PARTY',
  state: 'DRAFT',
  created_by: '',
  version: 1,
  contract_version: 1,
}

const hasContractTasks = computed(() => props.tasks.some((task) => task.type === 'contract'))
const hasTemplateTasks = computed(() => props.tasks.some((task) => task.type === 'template'))

const searchableItems = computed(() => {
  const items: Searchable[] = []
  const seenDids = new Set<string>()

  for (const task of props.tasks) {
    if (seenDids.has(task.did)) continue
    seenDids.add(task.did)

    if (task.type === 'template') {
      const template = templatesStore.findTemplateByDid(task.did)
      if (template) items.push(template)
    } else {
      const contract = contractsStore.findContractByDid(task.did)
      if (contract) items.push(contract)
    }
  }
  return items
})

function mapContractHits(response: ContractSearchResponse): Searchable[] {
  return response.map((item) => ({
    did: item.did,
    name: item.name,
    description: item.description,
    contract_version: item.contract_version,
    state: item.state,
    updated_at: item.updated_at,
    created_at: item.created_at,
    created_by: '',
  }))
}

function mapTemplateHits(response: ContractTemplateSearchResponse): Searchable[] {
  return response.map((item) => ({
    did: item.did,
    name: item.name,
    description: item.description,
    version: parseInt(item.version, 10),
    state: item.state,
    updated_at: item.updated_at,
    created_at: item.created_at,
    template_type: item.template_type,
    party_type: item.party_type,
    created_by: item.created_by,
  }))
}

function toContractSearchRequest(request: Record<string, unknown>): ContractSearchRequest {
  const next: ContractSearchRequest = { ...props.contractSearchDefaults }
  if (typeof request.did === 'string') next.did = request.did
  if (typeof request.name === 'string') next.name = request.name
  if (typeof request.description === 'string') next.description = request.description
  if (typeof request.contract_data === 'string') next.contract_data = request.contract_data
  if (typeof request.party === 'string') next.party = request.party
  if (typeof request.template_did === 'string') next.template_did = request.template_did
  if (typeof request.jurisdiction === 'string') next.jurisdiction = request.jurisdiction
  if (typeof request.contract_version === 'number') next.contract_version = request.contract_version
  if (typeof request.contract_version === 'string' && request.contract_version.trim() !== '') {
    const parsed = Number.parseInt(request.contract_version, 10)
    if (!Number.isNaN(parsed)) next.contract_version = parsed
  }
  return next
}

function toTemplateSearchRequest(request: Record<string, unknown>): ContractTemplateSearchRequest {
  const next: ContractTemplateSearchRequest = {}
  if (typeof request.did === 'string') next.did = request.did
  if (typeof request.name === 'string') next.name = request.name
  if (typeof request.description === 'string') next.description = request.description
  return next
}

function isContractOnlyFilter(request: Record<string, unknown>): boolean {
  return contractOnlyFilters.some((key) => Object.prototype.hasOwnProperty.call(request, key))
}

const search = async (request: Record<string, unknown>): Promise<Searchable[]> => {
  const byDid = new Map<string, Searchable>()

  if (hasContractTasks.value) {
    const hits = mapContractHits(await contractWorkflowService.search(toContractSearchRequest(request)))
    for (const hit of hits) byDid.set(hit.did, hit)
  }

  if (hasTemplateTasks.value && !isContractOnlyFilter(request)) {
    const hits = mapTemplateHits(await contractTemplateService.search(toTemplateSearchRequest(request)))
    for (const hit of hits) {
      if (!byDid.has(hit.did)) byDid.set(hit.did, hit)
    }
  }

  return [...byDid.values()]
}

const handleSearchResult = (searchResults: Searchable[] | null) => {
  if (!searchResults) return emit('searchResult', null)

  const resultDids = new Set(searchResults.map((item) => item.did))
  emit(
    'searchResult',
    props.tasks.filter((task) => resultDids.has(task.did)),
  )
}
</script>

<template>
  <ListSearch
    :items="searchableItems"
    :filter-labels="filterLabels"
    :search-fn="search"
    :empty-item="emptyItem"
    :placeholder="placeholder || 'Search templates/contracts'"
    @search-result="handleSearchResult"
  />
</template>
