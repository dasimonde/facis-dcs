import { defineStore } from 'pinia'
import { computed, type Ref, ref } from 'vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { TemplateState } from '@/types/contract-template-state'
import { compareValues } from '@/utils/comparison'
import type { Contract } from '@/models/contract/contract'
import type { ContractApprovalTask } from '@/models/contract/contract-approval-task'
import type { ContractNegotiationTask } from '@/models/contract/contract-negotiation-task'
import type { ContractReviewTask } from '@/models/contract/contract-review-task'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

export const useContractsStore = defineStore('contracts', () => {
  const contracts: Ref<Contract[]> = ref([])
  const paginatedContracts: Ref<Contract[]> = ref([])
  const reviewTasks: Ref<ContractReviewTask[]> = ref([])
  const approvalTasks: Ref<ContractApprovalTask[]> = ref([])
  const negotiationTasks: Ref<ContractNegotiationTask[]> = ref([])

  const loading = ref(false)
  const error = ref<string | null>(null)

  const contractTemplates: Ref<PartialContractTemplate[]> = ref([])

  const hasContracts = computed(() => contracts.value.length > 0)

  const hasApprovedTemplates = computed(() =>
    contractTemplates.value.some(
      (template) => template.state === TemplateState.registered || template.state === TemplateState.published,
    ),
  )
  const approvedTemplates = computed(() =>
    contractTemplates.value
      .filter((template) => template.state === TemplateState.registered || template.state === TemplateState.published)
      .sort((a, b) => compareValues(a, b, 'updated_at', 1)),
  )

  const findContractByDid = (did: string) => contracts.value.find((contract) => contract.did === did)

  const fetchContracts = async (limit?: number, offset?: number) =>
    await contractWorkflowService.retrieve({ limit, offset })

  async function loadContracts() {
    loading.value = true
    error.value = null
    try {
      const data = await fetchContracts()
      contracts.value = data.contracts
      reviewTasks.value = data.review_tasks.map((task) => ({ ...task, type: 'contract' }))
      approvalTasks.value = data.approval_tasks.map((task) => ({ ...task, type: 'contract' }))
      negotiationTasks.value = data.negotiation_tasks.map((task) => ({ ...task, type: 'contract' }))
    } catch (err: unknown) {
      error.value = err instanceof Error && err.message ? err.message : 'Error loading the contracts'
    } finally {
      loading.value = false
    }
  }

  async function loadApprovedTemplates() {
    loading.value = true
    error.value = null
    try {
      contractTemplates.value = await contractWorkflowService.retrieveApprovedTemplates()
    } catch (err: unknown) {
      error.value = err instanceof Error && err.message ? err.message : 'Error loading the templates'
    } finally {
      loading.value = false
    }
  }

  async function loadPaginatedContracts(currentPage: number, limit: number) {
    loading.value = true
    error.value = null
    try {
      const offset = currentPage
      const paginatedResult = await fetchContracts(limit, offset)
      paginatedContracts.value = paginatedResult.contracts
      negotiationTasks.value = paginatedResult.negotiation_tasks.map((task) => ({ ...task, type: 'contract' }))
      reviewTasks.value = paginatedResult.review_tasks.map((task) => ({ ...task, type: 'contract' }))
      approvalTasks.value = paginatedResult.approval_tasks.map((task) => ({ ...task, type: 'contract' }))
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Error loading contracts'
    } finally {
      loading.value = false
    }
  }

  const loadTasks = loadContracts

  function hasNegotiationTask(contract: Contract) {
    return negotiationTasks.value.some((task) => task.did === contract.did)
  }

  function hasReviewTask(contract: Contract) {
    return reviewTasks.value.some((task) => task.did === contract.did)
  }

  function hasApprovalTask(contract: Contract) {
    return approvalTasks.value.some((task) => task.did === contract.did)
  }

  return {
    contracts,
    reviewTasks,
    approvalTasks,
    negotiationTasks,
    hasContracts,
    paginatedContracts,
    findContractByDid,
    loadContracts,
    loadPaginatedContracts,
    loadTasks,
    loading,
    error,
    hasNegotiationTask,
    hasReviewTask,
    hasApprovalTask,
    loadApprovedTemplates,
    approvedTemplates,
    hasApprovedTemplates,
  }
})
