import type { Contract } from '@/models/contract/contract'
import type { ContractApprovalTask } from '@/models/contract/contract-approval-task'
import type { ContractNegotiationTask } from '@/models/contract/contract-negotiation-task'
import type { ContractReviewTask } from '@/models/contract/contract-review-task'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { defineStore } from 'pinia'
import { computed, ref, type Ref } from 'vue'

export const useContractsStore = defineStore('contracts', () => {
  const contracts: Ref<Contract[]> = ref([])
  const reviewTasks: Ref<ContractReviewTask[]> = ref([])
  const approvalTasks: Ref<ContractApprovalTask[]> = ref([])
  const negotiationTasks: Ref<ContractNegotiationTask[]> = ref([])

  const loading = ref(false)
  const error = ref<string | null>(null)

  const hasContracts = computed(() => contracts.value.length > 0)

  const findContractByDid = (did: string) => contracts.value.find((contract) => contract.did === did)

  async function loadContracts() {
    loading.value = true
    error.value = null
    try {
      const data = await contractWorkflowService.retrieve()
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
    findContractByDid,
    loadContracts,
    loading,
    error,
    hasNegotiationTask,
    hasReviewTask,
    hasApprovalTask,
  }
})
