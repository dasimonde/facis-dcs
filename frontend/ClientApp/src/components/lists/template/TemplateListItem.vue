<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import { useTemplatePermissions } from '@/modules/template-repository/composables/useTemplatePermissions'
import { ROUTES } from '@/router/router'
import { TemplateState } from '@/types/contract-template-state'
import { toProperCase } from '@/utils/string'
import { computed } from 'vue'

const props = defineProps<{
  template: PartialContractTemplate
}>()

const { isCreator, isReviewer, isApprover, isManager } = useTemplatePermissions()

const canEdit = computed(() => {
  const inDraftOrRejectedState =
    (props.template.state === TemplateState.draft || props.template.state === TemplateState.rejected) && isCreator.value
  const inSubmittedState = props.template.state === TemplateState.submitted && isReviewer.value
  const inValidStateForManager =
    (props.template.state === TemplateState.draft ||
      props.template.state === TemplateState.submitted ||
      props.template.state === TemplateState.rejected ||
      props.template.state === TemplateState.reviewed ||
      props.template.state === TemplateState.approved ||
      props.template.state === TemplateState.deleted) &&
    isManager.value
  return inDraftOrRejectedState || inSubmittedState || inValidStateForManager
})

const canReview = computed(() => {
  return props.template.state === TemplateState.submitted && isReviewer
})

const resolveViewRouteName = computed(() => {
  if (canReview.value) {
    return ROUTES.TEMPLATES.REVIEW
  }
  if (props.template.state === TemplateState.reviewed && isApprover) {
    return ROUTES.TEMPLATES.APPROVE
  }
  return ROUTES.TEMPLATES.VIEW
})

function getTemplateLink(template: PartialContractTemplate): string {
  return `/ui/templates/view/${template.latest_did}`
}
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <div class="-mt-9 mr-1 -ml-1 grid w-full grid-cols-3 items-center">
          <div class="badge justify-self-start badge-md badge-accent">{{ toProperCase(template.template_type) }}</div>
          <a
            v-if="template?.latest_did"
            class="badge justify-self-center badge-md badge-warning"
            :href="getTemplateLink(template)"
          >
            A newer template version is available
          </a>
          <div></div>
        </div>

        <h2 class="card-title items-start justify-between">
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <div class="truncate">{{ template.name }}</div>
          </div>
          <div class="ml-10 flex shrink-0 flex-col items-end">
            <div class="badge badge-secondary">{{ template.state }}</div>
          </div>
        </h2>
        <div class="flex flex-col">
          <div v-if="template.version">Version: {{ template.version }}</div>
          <div v-if="template.document_number">Document number: {{ template.document_number }}</div>
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
