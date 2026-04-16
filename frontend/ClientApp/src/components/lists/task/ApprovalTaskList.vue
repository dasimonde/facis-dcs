<script setup lang="ts">
import type { ContractTemplateApprovalTask } from '@/models/contract-template-approval-task'
import type { ContractApprovalTask } from '@/models/contract/contract-approval-task'
import { ROUTES } from '@/router/router'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useApprovalTaskStateFilterStore } from '@/stores/state-filter-store'
import { ApprovalTaskState, approvalTaskStates } from '@/types/approval-task-state'
import { TemplateState } from '@/types/contract-template-state'
import { compareValues } from '@/utils/comparison'
import { toProperCase } from '@/utils/string'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import TaskListSearch from './TaskListSearch.vue'

type ApprovalTask = ContractTemplateApprovalTask | ContractApprovalTask

const props = defineProps<{
  items: ApprovalTask[]
}>()

const templatesStore = useContractTemplatesStore()
const contractsStore = useContractsStore()
const stateFilterStore = useApprovalTaskStateFilterStore()

const sorter = new Map<keyof ApprovalTask, string>([
  ['created_at', 'Creation date'],
  ['state', 'Task state'],
])
const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const searchedItems: Ref<ApprovalTask[]> = ref([])
const isSearchActive = ref(false)

const displayedItems = computed(() => {
  return isSearchActive.value ? searchedItems.value : props.items
})

const sortedItems = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return displayedItems.value
  }
  return displayedItems.value.slice().sort((taskA, taskB) => compareValues(taskA, taskB, sortBy.value, sortOrder.value))
})

const filteredItems = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedItems.value.filter((item) => stateFilterStore.hasFilter(item.state))
  }
  return sortedItems.value
})

const getTemplateName = (item: ContractTemplateApprovalTask) => {
  return templatesStore.contractTemplates.find((template) => template.did === item.did)?.name ?? 'Nameless Template'
}

const getContractName = (item: ContractApprovalTask) => {
  return contractsStore.contracts.find((contract) => contract.did === item.did)?.name ?? 'Nameless Contract'
}

const getTemplateState = (item: ContractTemplateApprovalTask) => {
  return templatesStore.contractTemplates.find((template) => template.did === item.did)?.state
}

const canApprove = (item: ContractTemplateApprovalTask) => {
  return item.state === ApprovalTaskState.open && getTemplateState(item) === TemplateState.reviewed
}

const resolveViewRouteName = (item: ApprovalTask) => {
  if (item.type === 'template') {
    if (canApprove(item)) {
      return ROUTES.TEMPLATES.APPROVE
    }
    return ROUTES.TEMPLATES.VIEW
  } else {
    // TODO: contract view routes
    return ROUTES.CONTRACTS.VIEW
  }
}

const applySearchResult = (searchResult: ApprovalTask[]) => {
  isSearchActive.value = searchResult.length !== props.items.length
  searchedItems.value = searchResult
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <ul class="list">
    <li class="tracking-wide px-4 flex justify-end flex-col sm:flex-row">
      <ListStateFilter label="Approval Task" :filters="approvalTaskStates" store-type="approvalTasks" />
      <TaskListSearch class="flex-1" :items="items" @search-result="applySearchResult" />
      <ListSort :sorter="sorter" v-model:sort-by="sortBy" v-model:sort-order="sortOrder" />
    </li>
    <template v-if="filteredItems.length > 0">
      <li v-for="item in filteredItems" :key="item.did" class="list-row">
        <div class="list-col-grow card bg-base-200 card-border hover:bg-base-300">
          <div class="card-body">
            <h2 class="card-title flex-wrap justify-between">
              <div v-if="item.type === 'template'">Approval Task for Template: {{ getTemplateName(item) }}</div>
              <div v-else>Approval Task for Contract: {{ getContractName(item) }}</div>
              <div class="flex-1"></div>
              <div class="badge badge-accent">{{ toProperCase(item.type) }} Task</div>
              <div class="badge badge-secondary">{{ item.state }}</div>
            </h2>
            <div class="flex justify-between">
              <div v-if="item.type === 'template' && item.document_number">
                Document number: {{ item.document_number }}
              </div>
              <div v-if="item.type === 'template' && item.version">Version: {{ item.version }}</div>
              <div v-else-if="item.type === 'contract' && item.contract_version">
                Version: {{ item.contract_version }}
              </div>
            </div>
            <div class="flex justify-between">
              <div>Creation date: {{ new Date(item.created_at).toLocaleDateString() }}</div>
              <div class="card-actions justify-end">
                <RouterLink
                  :to="{
                    name: resolveViewRouteName(item),
                    params: { did: item.did },
                  }"
                  class="btn btn-sm btn-primary rounded-box"
                >
                  View
                </RouterLink>
              </div>
            </div>
          </div>
        </div>
      </li>
    </template>
    <li v-else class="px-4">No approval tasks found.</li>
  </ul>
</template>
