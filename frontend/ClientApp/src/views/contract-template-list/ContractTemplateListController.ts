import type { PartialContractTemplate } from '@/models/contract-template'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import type { UserRole } from '@/types/user-role'
import { computed, onMounted, ref, type Ref } from 'vue'

export function useTemplateTable() {
    const templatesStore = useContractTemplatesStore()
    const templates = computed(() => templatesStore.contractTemplates)
    const reviewTasks = computed(() => templatesStore.reviewTasks)
    const approvalTasks = computed(() => templatesStore.approvalTasks)
    const roles: Ref<UserRole[]> = ref([])
    const loading = computed(() => templatesStore.loading)
    const error = computed(() => templatesStore.error)
    const authStore = useAuthStore()

    const loadTemplates = async () => {
        await templatesStore.loadTemplates()
        roles.value = authStore.user?.roles ?? []
    }

    const refresh = () => loadTemplates()  // Für manuelles Refresh

    const getTemplateById = async (did: string) => {
        try {
            return await contractTemplateService.retrieveById({ did })
        } catch (err: any) {
            console.error('Template konnte nicht geladen werden:', err)
            return null
        }
    }

    onMounted(loadTemplates)

    const hasReviewTask = (template: PartialContractTemplate): boolean => {
        const currentUser = authStore.user
        if (!currentUser) return false
        return reviewTasks.value.some((task) => {
            const isDidMatch = task.did === template.did
            const isVersionMatch = !template.version || task.version === template.version
            const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
            return (
                isDidMatch &&
                isVersionMatch &&
                isDocumentNumberMatch &&
                task.reviewer === currentUser.username
            )
        })
    }

    const hasApprovalTask = (template: PartialContractTemplate): boolean => {
        const currentUser = authStore.user
        if (!currentUser) return false
        return approvalTasks.value.some((task) => {
            const isDidMatch = task.did === template.did
            const isVersionMatch = !template.version || task.version === template.version
            const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
            return (
                isDidMatch &&
                isVersionMatch &&
                isDocumentNumberMatch &&
                task.approver === currentUser.username
            )
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
        getTemplateById,
        hasReviewTask,
        hasApprovalTask,
    }
}
