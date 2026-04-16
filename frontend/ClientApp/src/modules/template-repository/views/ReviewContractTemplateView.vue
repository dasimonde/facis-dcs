<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">

    <TemplateEditors title="Review Template" />

    <!-- Pinned Footer -->
    <div v-if="hasDid" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <!-- Comments container -->
      <ConfirmationModal ref="comment-dialog" show-editor />
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="router.back()">Cancel</button>
        <!-- Return to draft / request changes -->
        <button @click="returnToDraft" class="btn btn-primary flex-1" :disabled="isSubmitting">
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Return to draft
        </button>
        <!-- Complete review (verify then forward to approval) -->
        <button @click="forwardToApproval" class="btn btn-primary flex-1" :disabled="isSubmitting">
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Forward to approval
        </button>
        <TemplateManagerActions v-if="contractTemplate && isManager" :item="contractTemplate" class="btn btn-primary flex-1" />
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, type Ref, useTemplateRef } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import TemplateManagerActions from '@/components/lists/template/TemplateManagerActions.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { contractTemplateService } from '@/services/contract-template-service'

const router = useRouter()
const route = useRoute()

const authStore = useAuthStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()

const commentDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('comment-dialog')

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const isManager = computed(() => {
  return hasDid.value && (authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false)
})

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

watch(hasDid, (hasDidVal) => {
  templateEditorUiStore.reset()
  if (!hasDidVal) return

  hasChosenType.value = true
  const did = `${route.params.did}`
  contractTemplateService.retrieveById({ did })
    .then(template => {
      if (!template) {
        draftStore.reset()
        return
      }
      templateEditorUiStore.setTemplateEditable(false)
      contractTemplate.value = template

      draftStore.reset({
        did: template.did,
        name: template.name,
        description: template.description,
        documentOutline: template.template_data?.documentOutline ?? [],
        documentBlocks: template.template_data?.documentBlocks ?? [],
        semanticConditions: template.template_data?.semanticConditions ?? [],
        customMetaData: template.template_data?.customMetaData ?? [],
        subTemplateSnapshots: template.template_data?.subTemplateSnapshots ?? [],
        templateType: template.template_type,
        state: template.state,
        version: template.version ?? null,
        document_number: template.document_number ?? null,
        updated_at: template.updated_at ?? null,
      })
    })
    .catch(error => {
      console.error('Failed to load template for editing', error)
    })

}, { immediate: true })

const isSubmitting = ref(false)
const comment = ref<string>('')

const forwardToApproval = async () => {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    console.error('Missing did or updated_at for submission')
    return
  }
  isSubmitting.value = true
  try {
    const commentResult = await commentDialog.value?.reveal({ message: 'Add comment?' })
    if (commentResult?.isCanceled) {
      return
    } else if (commentResult?.data) {
      comment.value = commentResult.data
    }
    await contractTemplateService.verify({
      did,
    })
    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: comment.value ? [comment.value] : [],
      forward_to: 'APPROVAL',
      approver: '',
      reviewers: [],
    })
    router.push({ name: ROUTES.TEMPLATES.LIST })
  } catch (error) {
    console.error('Submission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

const returnToDraft = async () => {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    console.error('Missing did or updated_at for rejection')
    return
  }
  isSubmitting.value = true
  try {
    const commentResult = await commentDialog.value?.reveal({ message: 'Add comment?' })
    if (commentResult?.isCanceled) {
      return
    } else if (commentResult?.data) {
      comment.value = commentResult.data
    }
    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: comment.value ? [comment.value] : [],
      forward_to: 'DRAFT',
      approver: '',
      reviewers: [],
    })
    router.push({ name: ROUTES.TEMPLATES.LIST })
  } catch (error) {
    console.error('Rejection failed', error)
  } finally {
    isSubmitting.value = false
  }
}

</script>
