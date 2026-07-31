import { defineStore } from 'pinia'
import { type Ref, ref } from 'vue'
import { contractTemplateService } from '@/services/contract-template-service'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'
import type { ContractTemplateApprovalTask } from '@/models/contract-template/contract-template-approval-task'
import type { ContractTemplateReviewTask } from '@/models/contract-template/contract-template-review-task'

export const useContractTemplatesStore = defineStore('contractTemplates', () => {
  const contractTemplates: Ref<PartialContractTemplate[]> = ref([])
  const paginatedTemplates: Ref<PartialContractTemplate[]> = ref([])
  const reviewTasks: Ref<ContractTemplateReviewTask[]> = ref([])
  const approvalTasks: Ref<ContractTemplateApprovalTask[]> = ref([])

  const loading = ref(false)
  const error = ref<string | null>(null)

  const findTemplateByDid = (did: string) => contractTemplates.value.find((template) => template.did === did)

  const fetchTemplates = async (limit?: number, offset?: number) =>
    await contractTemplateService.retrieve({ limit, offset })

  async function loadTemplates() {
    loading.value = true
    error.value = null
    try {
      const data = await fetchTemplates()
      contractTemplates.value = data.contract_templates
      reviewTasks.value = data.review_tasks.map((task) => ({ ...task, type: 'template' }))
      approvalTasks.value = data.approval_tasks.map((task) => ({ ...task, type: 'template' }))
    } catch (err: unknown) {
      error.value = err instanceof Error && err.message ? err.message : 'Error loading the templates'
    } finally {
      loading.value = false
    }
  }

  async function loadPaginatedTemplates(currentPage: number, limit: number) {
    loading.value = true
    error.value = null
    try {
      const offset = currentPage
      const paginatedResult = await fetchTemplates(limit, offset)
      paginatedTemplates.value = paginatedResult.contract_templates
      reviewTasks.value = paginatedResult.review_tasks.map((task) => ({ ...task, type: 'template' }))
      approvalTasks.value = paginatedResult.approval_tasks.map((task) => ({ ...task, type: 'template' }))
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Error loading templates'
    } finally {
      loading.value = false
    }
  }

  const loadTasks = loadTemplates

  return {
    contractTemplates,
    reviewTasks,
    approvalTasks,
    paginatedTemplates,
    findTemplateByDid,
    fetchTemplates,
    loadTemplates,
    loadPaginatedTemplates,
    loadTasks,
    loading,
    error,
  }
})
