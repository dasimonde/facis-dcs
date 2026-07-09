import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth-store'

export const useContractPermissions = () => {
  const authStore = useAuthStore()

  const isCreator = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_CREATOR') ?? false
  })

  const isReviewer = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_REVIEWER') ?? false
  })

  const isApprover = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_APPROVER') ?? false
  })

  const isManager = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_MANAGER') ?? false
  })

  const isSigner = computed(() => {
    return authStore.user?.roles?.includes('CONTRACT_SIGNER') ?? false
  })

  return { isCreator, isReviewer, isApprover, isManager, isSigner }
}
