<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <TemplateEditors title="Approve Template" />

    <!-- Pinned Footer -->
    <div v-if="hasDid" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <!-- Decision notes container -->
      <ConfirmationModal ref="decision-note-dialog" />
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="router.back()">Cancel</button>
        <CopyTemplateButton v-if="isCreator || isManager" class="btn flex-1 btn-primary" />
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="reject">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Reject
        </button>
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="resubmit">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Resubmit
        </button>
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="approve">
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

const decisionNoteDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('decision-note-dialog')

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const { isCreator, isManager: isManagerBase } = useTemplatePermissions()
const isManager = computed(() => hasDid.value && isManagerBase.value)

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

watch(
  hasDid,
  (hasDid) => {
    templateEditorUiStore.reset()
    if (!hasDid) return

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
const decisionNote = ref<string>('')

async function approve() {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    console.error('Missing did or updated_at for approval')
    return
  }
  isSubmitting.value = true
  try {
    const decisionNoteResult = await decisionNoteDialog.value?.reveal({
      message: 'Add decision note?',
      editor: { requiredText: false, placeholder: 'Decision Note' },
    })
    if (decisionNoteResult?.isCanceled) {
      return
    } else if (decisionNoteResult?.data) {
      decisionNote.value = decisionNoteResult.data
    }
    await contractTemplateService.approve({
      did,
      updated_at: updatedAt,
      decision_notes: decisionNote.value ? [decisionNote.value] : [],
    })
    await navStore.goToPreviousRoute()
  } catch (error) {
    console.error('Approval failed', error)
  } finally {
    isSubmitting.value = false
  }
}

async function resubmit() {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    console.error('Missing did or updated_at for reopen reviews')
    return
  }
  isSubmitting.value = true
  try {
    const decisionNoteResult = await decisionNoteDialog.value?.reveal({
      message: 'Add decision note?',
      editor: { requiredText: false, placeholder: 'Decision Note' },
    })
    if (decisionNoteResult?.isCanceled) {
      return
    } else if (decisionNoteResult?.data) {
      decisionNote.value = decisionNoteResult.data
    }
    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: decisionNote.value ? [decisionNote.value] : [],
    })
    await navStore.goToPreviousRoute()
  } catch (error) {
    console.error('Resubmission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

async function reject() {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    console.error('Missing did or updated_at for rejection')
    return
  }
  const decisionNoteResult = await decisionNoteDialog.value?.reveal({
    message: 'Add reason:',
    editor: { requiredText: true, placeholder: 'Decision Note' },
  })
  if (decisionNoteResult?.isCanceled) {
    return
  } else if (decisionNoteResult?.data) {
    decisionNote.value = decisionNoteResult.data
  }
  if (!decisionNote.value?.trim()) {
    console.error('Reason is required for rejection')
    return
  }
  isSubmitting.value = true
  try {
    await contractTemplateService.reject({
      did,
      updated_at: updatedAt,
      reason: decisionNote.value.trim(),
    })
    await navStore.goToPreviousRoute()
  } catch (error) {
    console.error('Rejection failed', error)
  } finally {
    isSubmitting.value = false
  }
}
</script>
