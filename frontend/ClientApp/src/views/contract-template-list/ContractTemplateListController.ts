import type { PartialContractTemplate } from '@/models/contract-template'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import type { UserRole } from '@/types/user-role'
import { storeToRefs } from 'pinia'
import { onMounted, ref, type Ref } from 'vue'

export function useTemplateList() {
  const templatesStore = useContractTemplatesStore()
  const { contractTemplates: templates, reviewTasks, approvalTasks, loading, error } = storeToRefs(templatesStore)
  const roles: Ref<UserRole[]> = ref([])
  const authStore = useAuthStore()

  const loadTemplates = async () => {
    await templatesStore.loadTemplates()
    roles.value = authStore.user?.roles ?? []
  }

  const refresh = () => loadTemplates() // Für manuelles Refresh

  onMounted(loadTemplates)

  const hasReviewTask = (template: PartialContractTemplate): boolean => {
    const currentUser = authStore.user
    if (!currentUser) return false
    return reviewTasks.value.some((task) => {
      const isDidMatch = task.did === template.did
      const isVersionMatch = !template.version || task.version === template.version
      const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
      return isDidMatch && isVersionMatch && isDocumentNumberMatch && task.reviewer === currentUser.username
    })
  }

  const hasApprovalTask = (template: PartialContractTemplate): boolean => {
    const currentUser = authStore.user
    if (!currentUser) return false
    return approvalTasks.value.some((task) => {
      const isDidMatch = task.did === template.did
      const isVersionMatch = !template.version || task.version === template.version
      const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
      return isDidMatch && isVersionMatch && isDocumentNumberMatch && task.approver === currentUser.username
    })
  }

  return {
    templates,
    reviewTasks,
    approvalTasks,
    roles,
    loading,
    error,
    loadTemplates,
    refresh,
    hasReviewTask,
    hasApprovalTask,
  }
}
