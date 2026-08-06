<script setup lang="ts">
import { computed, onUnmounted, type Ref, ref } from 'vue'
import { ROUTES } from '@/router/router'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useApprovalTaskStateFilterStore } from '@/stores/state-filter-store'
import { ApprovalTaskState, approvalTaskStates } from '@/types/approval-task-state'
import { ContractState } from '@/types/contract-state'
import { TemplateState } from '@/types/contract-template-state'
import { compareValues } from '@/utils/comparison'
import { toProperCase } from '@/utils/string'
import TaskListSearch from './TaskListSearch.vue'
import ListSort from '../ListSort.vue'
import ListStateFilter from '../ListStateFilter.vue'
import type { ContractApprovalTask } from '@/models/contract/contract-approval-task'
import type { ContractTemplateApprovalTask } from '@/models/contract-template/contract-template-approval-task'

type ApprovalTask = ContractTemplateApprovalTask | ContractApprovalTask

const props = defineProps<{
  tasks: ApprovalTask[]
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

const searchedTasks: Ref<ApprovalTask[]> = ref([])
const isSearchActive = ref(false)

const displayedTasks = computed(() => {
  return isSearchActive.value ? searchedTasks.value : props.tasks
})

const sortedTasks = computed(() => {
  if (!sorter.has(sortBy.value)) {
    return displayedTasks.value
  }
  return displayedTasks.value.slice().sort((taskA, taskB) => compareValues(taskA, taskB, sortBy.value, sortOrder.value))
})

const hasTasks = computed(() => props.tasks.length > 0)

const filteredTasks = computed(() => {
  if (stateFilterStore.hasFilters) {
    return sortedTasks.value.filter((task) => stateFilterStore.hasFilter(task.state))
  }
  return sortedTasks.value.filter(canApprove)
})

const getTemplateName = (task: ContractTemplateApprovalTask) => {
  return templatesStore.findTemplateByDid(task.did)?.name ?? 'Nameless Template'
}

const getContractName = (task: ContractApprovalTask) => {
  return contractsStore.findContractByDid(task.did)?.name ?? 'Nameless Contract'
}

const getTemplateState = (task: ContractTemplateApprovalTask) => {
  return templatesStore.findTemplateByDid(task.did)?.state
}

const getContractState = (task: ContractApprovalTask) => {
  return contractsStore.findContractByDid(task.did)?.state
}

const canApprove = (task: ApprovalTask) => {
  if (task.type === 'template') {
    return task.state === ApprovalTaskState.open && getTemplateState(task) === TemplateState.reviewed
  }
  return task.state === ApprovalTaskState.open && getContractState(task) === ContractState.reviewed
}

const resolveViewRouteName = (task: ApprovalTask) => {
  if (task.type === 'template') {
    if (canApprove(task)) {
      return ROUTES.TEMPLATES.APPROVE
    }
    return ROUTES.TEMPLATES.VIEW
  } else {
    if (canApprove(task)) {
      return ROUTES.CONTRACTS.APPROVE
    }
    return ROUTES.CONTRACTS.VIEW
  }
}

const applySearchResult = (searchResult: ApprovalTask[] | null) => {
  isSearchActive.value = !!searchResult
  if (searchResult) {
    searchedTasks.value = searchResult
  }
}

onUnmounted(() => stateFilterStore.reset())
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <ul class="list flex-1 overflow-y-auto">
      <li class="flex flex-col justify-end px-4 tracking-wide sm:flex-row">
        <TaskListSearch class="flex-1" :tasks="tasks" @search-result="applySearchResult" />
        <ListStateFilter
          label="Approval Task"
          :filters="approvalTaskStates"
          store-type="approvalTasks"
          :disabled="!hasTasks"
        />
        <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" :disabled="!hasTasks" />
      </li>
      <template v-if="filteredTasks.length > 0">
        <li v-for="task in filteredTasks" :key="task.did" class="list-row">
          <div class="list-col-grow card border-base-content/10 bg-base-100 card-border hover:bg-base-300">
            <div class="card-body">
              <h2 class="card-title flex-wrap justify-between">
                <div v-if="task.type === 'template'">Template Name: {{ getTemplateName(task) }}</div>
                <div v-else>Contract Name: {{ getContractName(task) }}</div>
                <div class="flex-1"></div>
                <div class="badge badge-accent">{{ toProperCase(task.type) }} Task</div>
                <div class="badge badge-secondary">{{ task.state }}</div>
              </h2>
              <div class="flex justify-between">
                <div v-if="task.type === 'template' && task.version">Version: {{ task.version }}</div>
                <div v-else-if="task.type === 'contract' && task.contract_version">
                  Version: {{ task.contract_version }}
                </div>
              </div>
              <div class="flex justify-between">
                <div>Creation date: {{ new Date(task.created_at).toLocaleDateString() }}</div>
                <div class="card-actions justify-end">
                  <RouterLink
                    :to="{
                      name: resolveViewRouteName(task),
                      params: { did: task.did },
                    }"
                    class="btn btn-sm btn-primary"
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
  </div>
</template>
