import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth-store'

export const useTemplatePermissions = () => {
  const authStore = useAuthStore()

  const isCreator = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_CREATOR') ?? false
  })

  const isReviewer = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_REVIEWER') ?? false
  })

  const isApprover = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_APPROVER') ?? false
  })

  const isManager = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
  })

  // The template list and history endpoints accept only these four roles. A
  // Contract Creator reads the one template it instantiates by its ID.
  const hasTemplateRole = computed(() => isCreator.value || isReviewer.value || isApprover.value || isManager.value)

  // A Contract Creator verifies the template it uses and reads its provenance.
  const isContractCreator = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_CREATOR') ?? false
  })

  return { isCreator, isReviewer, isApprover, isManager, hasTemplateRole, isContractCreator }
}
