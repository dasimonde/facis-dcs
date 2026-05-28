<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { TemplateState } from '@/types/contract-template-state'
import { toProperCase } from '@/utils/string'
import { computed } from 'vue'

const props = defineProps<{
  template: PartialContractTemplate
  hasReviewTask: boolean
  hasApprovalTask: boolean
}>()

const authStore = useAuthStore()
const templateStore = useContractTemplatesStore()

const canEdit = computed(() => {
  return (
    (props.template.created_by === authStore.user?.username &&
      (props.template.state === TemplateState.draft || props.template.state === TemplateState.rejected)) ||
    (props.template.state === TemplateState.submitted && props.hasReviewTask)
  )
})

const canReview = computed(() => {
  const task = templateStore.reviewTasks.find((task) => task.did === props.template.did)
  return props.template.state === TemplateState.submitted && props.hasReviewTask && !!task && task.state !== 'APPROVED'
})

const resolveViewRouteName = computed(() => {
  if (canReview.value) {
    return ROUTES.TEMPLATES.REVIEW
  }
  if (props.template.state === TemplateState.reviewed && props.hasApprovalTask) {
    return ROUTES.TEMPLATES.APPROVE
  }
  return ROUTES.TEMPLATES.VIEW
})
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ template.name }}</div>
            <div class="badge badge-accent sm:h-full sm:badge-md">{{ toProperCase(template.template_type) }}</div>
          </div>
          <div class="badge badge-secondary">{{ template.state }}</div>
        </h2>
        <div class="flex justify-between">
          <div v-if="template.document_number">Document number: {{ template.document_number }}</div>
          <div v-if="template.version">Version: {{ template.version }}</div>
        </div>
        <div class="flex min-w-0 justify-between">
          <div>Creation date: {{ new Date(template.created_at).toLocaleDateString() }}</div>
          <div v-if="template.description" class="hidden min-w-0 flex-1 truncate px-10 sm:block">
            {{ template.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="{ name: resolveViewRouteName, params: { did: template.did } }"
              class="btn btn-sm btn-primary"
            >
              View
            </RouterLink>
            <RouterLink
              :to="
                canEdit
                  ? {
                      name: ROUTES.TEMPLATES.EDIT,
                      params: { did: template.did },
                    }
                  : '#'
              "
              class="btn gap-2 btn-sm btn-primary"
              :class="{ 'btn-disabled': !canEdit }"
            >
              Edit
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </li>
</template>
