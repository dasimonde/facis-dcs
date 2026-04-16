<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { TemplateState } from '@/types/contract-template-state'
import { toProperCase } from '@/utils/string'
import { computed } from 'vue'

const props = defineProps<{
  item: PartialContractTemplate
  hasReviewTask: boolean
  hasApprovalTask: boolean
}>()

const authStore = useAuthStore()
const templateStore = useContractTemplatesStore()

const canEdit = computed(() => {
  return (
    (props.item.created_by === authStore.user?.username &&
      (props.item.state === TemplateState.draft || props.item.state === TemplateState.rejected)) ||
    (props.item.state === TemplateState.submitted && props.hasReviewTask)
  )
})

const canReview = computed(() => {
  const task = templateStore.reviewTasks.find((task) => task.did === props.item.did)
  return props.item.state === TemplateState.submitted && props.hasReviewTask && !!task && task.state !== 'APPROVED'
})

const resolveViewRouteName = computed(() => {
  if (canReview.value) {
    return ROUTES.TEMPLATES.REVIEW
  }
  if (props.item.state === TemplateState.reviewed && props.hasApprovalTask) {
    return ROUTES.TEMPLATES.APPROVE
  }
  return ROUTES.TEMPLATES.VIEW
})
</script>

<template>
  <li class="list-row min-w-0 w-full">
    <div class="list-col-grow card bg-base-200 card-border hover:bg-base-300 min-w-0 w-full">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ item.name }}</div>
            <div class="badge sm:badge-md badge-accent sm:h-full">{{ toProperCase(item.template_type) }}</div>
          </div>
          <div class="badge badge-secondary">{{ item.state }}</div>
        </h2>
        <div class="flex justify-between">
          <div v-if="item.document_number">Document number: {{ item.document_number }}</div>
          <div v-if="item.version">Version: {{ item.version }}</div>
        </div>
        <div class="flex justify-between min-w-0">
          <div>Creation date: {{ new Date(item.created_at).toLocaleDateString() }}</div>
          <div v-if="item.description" class="px-10 flex-1 min-w-0 truncate hidden sm:block">
            {{ item.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="{ name: resolveViewRouteName, params: { did: item.did } }"
              class="btn btn-sm btn-primary rounded-box"
            >
              View
            </RouterLink>
            <RouterLink
              :to="canEdit ? {
                name: ROUTES.TEMPLATES.EDIT,
                params: { did: item.did },
              } : '#'"
              class="btn btn-sm btn-primary rounded-box gap-2"
              :class="{'btn-disabled': !canEdit}"
            >
              Edit
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </li>
</template>
