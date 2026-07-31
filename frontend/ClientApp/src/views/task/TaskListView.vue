<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import ApprovalTaskList from '@/components/lists/task/ApprovalTaskList.vue'
import NegotiationTaskList from '@/components/lists/task/NegotiationTaskList.vue'
import ReviewTaskList from '@/components/lists/task/ReviewTaskList.vue'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import type { UserRole } from '@/types/user-role'

const route = useRoute()

const authStore = useAuthStore()

const templatesStore = useContractTemplatesStore()
const contractsStore = useContractsStore()

const loading = computed(() => templatesStore.loading || contractsStore.loading)
const error = computed(() => templatesStore.error ?? contractsStore.error)

const reviewTasks = computed(() => {
  return route.name === ROUTES.TASKS.REVIEWS ? [...templatesStore.reviewTasks, ...contractsStore.reviewTasks] : []
})
const approvalTasks = computed(() => {
  return route.name === ROUTES.TASKS.APPROVALS ? [...templatesStore.approvalTasks, ...contractsStore.approvalTasks] : []
})
const negotiationTasks = computed(() => {
  return route.name === ROUTES.TASKS.NEGOTIATIONS ? contractsStore.negotiationTasks : []
})

const hasTemplateRole = computed(() => {
  return (
    authStore.user?.roles?.some((role) =>
      (['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER', 'TEMPLATE_APPROVER', 'TEMPLATE_MANAGER'] as UserRole[]).includes(role),
    ) ?? false
  )
})

const hasContractRole = computed(() => {
  return (
    authStore.user?.roles?.some((role) =>
      (
        [
          'CONTRACT_CREATOR',
          'CONTRACT_NEGOTIATOR',
          'CONTRACT_REVIEWER',
          'CONTRACT_APPROVER',
          'CONTRACT_MANAGER',
        ] as UserRole[]
      ).includes(role),
    ) ?? false
  )
})

const loadTasks = async () => {
  if (route.name !== ROUTES.TASKS.NEGOTIATIONS && hasTemplateRole.value) {
    await templatesStore.loadTasks()
  }
  if (hasContractRole.value) {
    await contractsStore.loadTasks()
  }
}

watch(
  () => route.name,
  () => loadTasks(),
  { immediate: true },
)
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <h2
      class="mb-4 shrink-0 border-b border-base-content/10 bg-base-100 p-4 text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight"
    >
      {{ route.meta.name }}
    </h2>

    <div v-if="loading" class="pl-4" role="status" aria-live="polite">Loading tasks...</div>
    <div v-else-if="error" class="pl-4" role="status" aria-live="assertive">{{ error }}</div>
    <div v-else class="min-h-0 flex-1 overflow-hidden">
      <template v-if="route.name === ROUTES.TASKS.REVIEWS">
        <ReviewTaskList :tasks="reviewTasks" />
      </template>
      <template v-else-if="route.name === ROUTES.TASKS.APPROVALS">
        <ApprovalTaskList :tasks="approvalTasks" />
      </template>
      <template v-else-if="route.name === ROUTES.TASKS.NEGOTIATIONS">
        <NegotiationTaskList :tasks="negotiationTasks" />
      </template>
    </div>
  </div>
</template>
