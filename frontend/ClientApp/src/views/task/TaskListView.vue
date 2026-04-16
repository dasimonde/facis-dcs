<script setup lang="ts">
import ApprovalTaskList from '@/components/lists/task/ApprovalTaskList.vue'
import NegotiationTaskList from '@/components/lists/task/NegotiationTaskList.vue'
import ReviewTaskList from '@/components/lists/task/ReviewTaskList.vue'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { useContractsStore } from '@/stores/contracts-store'
import type { UserRole } from '@/types/user-role'
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const authStore = useAuthStore()

const templatesStore = useContractTemplatesStore()
const contractsStore = useContractsStore()

const loading = computed(() => templatesStore.loading || contractsStore.loading)
const error = computed(() => templatesStore.error || contractsStore.error)

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
      (['CONTRACT_CREATOR', 'CONTRACT_REVIEWER', 'CONTRACT_APPROVER', 'CONTRACT_MANAGER'] as UserRole[]).includes(role),
    ) ?? false
  )
})

const loadTasks = async () => {
  if (route.name !== ROUTES.TASKS.NEGOTIATIONS && hasTemplateRole.value) {
    await templatesStore.loadTemplates()
  }
  if (hasContractRole.value) {
    await contractsStore.loadContracts()
  }
}

watch(
  () => route.name,
  () => loadTasks(),
  { immediate: true },
)
</script>

<template>
  <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight p-4 mb-4">
    {{ $route.meta.name }}
  </h2>

  <div v-if="loading">Loading Tasks...</div>
  <div v-else-if="error">{{ error }}</div>
  <template v-else>
    <template v-if="$route.name === ROUTES.TASKS.REVIEWS">
      <ReviewTaskList :items="reviewTasks" />
    </template>
    <template v-else-if="$route.name === ROUTES.TASKS.APPROVALS">
      <ApprovalTaskList :items="approvalTasks" />
    </template>
    <template v-else-if="$route.name === ROUTES.TASKS.NEGOTIATIONS">
      <NegotiationTaskList :items="negotiationTasks" />
    </template>
  </template>
</template>
