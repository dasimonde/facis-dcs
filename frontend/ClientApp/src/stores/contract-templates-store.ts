import type { PartialContractTemplate } from '@/models/contract-template'
import type { ContractTemplateApprovalTask } from '@/models/contract-template-approval-task'
import type { ContractTemplateReviewTask } from '@/models/contract-template-review-task'
import { contractTemplateService } from '@/services/contract-template-service'
import { TemplateState } from '@/types/contract-template-state'
import { defineStore } from 'pinia'
import { computed, ref, type Ref } from 'vue'
import { TemplateType } from '@template-repository/models/contract-templace.ts'

export const useContractTemplatesStore = defineStore('contractTemplates', () => {
  const contractTemplates: Ref<PartialContractTemplate[]> = ref([])
  const reviewTasks: Ref<ContractTemplateReviewTask[]> = ref([])
  const approvalTasks: Ref<ContractTemplateApprovalTask[]> = ref([])

  const loading = ref(false)
  const error = ref<string | null>(null)

  const hasTemplates = computed(() => contractTemplates.value.length > 0)
  const hasApprovedOrPublishedTemplates = computed(() =>
    contractTemplates.value.some(
      (template) =>
        (template.state === TemplateState.approved || template.state === TemplateState.published) &&
        template.template_type === TemplateType.frameContract,
    ),
  )
  const approvedOrPublishedTemplates = computed(() =>
    contractTemplates.value.filter(
      (template) =>
        (template.state === TemplateState.approved || template.state === TemplateState.published) &&
        template.template_type === TemplateType.frameContract,
    ),
  )

  const findTemplateByDid = (did: string) => contractTemplates.value.find((template) => template.did === did)

  async function loadTemplates() {
    loading.value = true
    error.value = null
    try {
      const data = await contractTemplateService.retrieve()
      contractTemplates.value = data.contract_templates
      reviewTasks.value = data.review_tasks.map((task) => ({ ...task, type: 'template' }))
      approvalTasks.value = data.approval_tasks.map((task) => ({ ...task, type: 'template' }))
    } catch (err: unknown) {
      error.value = err instanceof Error && err.message ? err.message : 'Error loading the templates'
    } finally {
      loading.value = false
    }
  }

  return {
    contractTemplates,
    reviewTasks,
    approvalTasks,
    hasTemplates,
    hasApprovedOrPublishedTemplates,
    approvedOrPublishedTemplates,
    findTemplateByDid,
    loadTemplates,
    loading,
    error,
  }
})
