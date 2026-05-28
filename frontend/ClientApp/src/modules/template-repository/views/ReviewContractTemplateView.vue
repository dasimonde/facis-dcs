<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <TemplateEditors title="Review Template" />

    <!-- Pinned Footer -->
    <div v-if="hasDid" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <!-- Comments container -->
      <ConfirmationModal ref="comment-dialog" />
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="router.back()">Cancel</button>
        <CopyTemplateButton v-if="isCreator || isManager" class="btn flex-1 btn-primary" />
        <!-- Return to draft / request changes -->
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="returnToDraft">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Reject
        </button>
        <!-- Complete review (verify then forward to approval) -->
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="forwardToApproval">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Approve
        </button>
        <TemplateManagerActions
          v-if="contractTemplate && isManager"
          :template="contractTemplate"
          class="btn flex-1 btn-primary"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import TemplateManagerActions from '@/components/template/TemplateManagerActions.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { contractTemplateService } from '@/services/contract-template-service'
import { useNavStore } from '@/stores/nav-store'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { computed, ref, useTemplateRef, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CopyTemplateButton from '../components/CopyTemplateButton.vue'

const router = useRouter()
const route = useRoute()
const navStore = useNavStore()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()

const commentDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('comment-dialog')

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const { isCreator, isManager: isManagerBase } = useTemplatePermissions()
const isManager = computed(() => hasDid.value && isManagerBase.value)

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

watch(
  hasDid,
  (hasDidVal) => {
    templateEditorUiStore.reset()
    if (!hasDidVal) return

    hasChosenType.value = true
    const did = String(route.params.did ?? '')
    contractTemplateService
      .retrieveById({ did })
      .then((template) => {
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
          templateDataVersion: template.template_data?.templateDataVersion ?? 1,
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
          responsible_persons: template.responsible_persons ?? null,
        })
      })
      .catch((error: unknown) => {
        console.error('Failed to load template for editing', error)
      })
  },
  { immediate: true },
)

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
    const commentResult = await commentDialog.value?.reveal({
      message: 'Add comment?',
      editor: { requiredText: false },
    })
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
    await navStore.goToPreviousRoute()
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
    const commentResult = await commentDialog.value?.reveal({
      message: 'Add comment?',
      editor: { requiredText: false },
    })
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
    await navStore.goToPreviousRoute()
  } catch (error) {
    console.error('Rejection failed', error)
  } finally {
    isSubmitting.value = false
  }
}
</script>
