<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">

    <TemplateEditors title="Approve Template" />

    <!-- Pinned Footer -->
    <div v-if="hasDid" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <!-- Decision notes container -->
      <ConfirmationModal ref="decision-note-dialog" show-editor editor-placeholder="Decision Note" />
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="router.back()">Cancel</button>
        <button @click="reject" class="btn btn-primary flex-1" :disabled="isSubmitting">
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Reject
        </button>
        <button @click="resubmit" class="btn btn-primary flex-1" :disabled="isSubmitting">
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Resubmission
        </button>
        <button @click="approve" class="btn btn-primary flex-1" :disabled="isSubmitting">
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Approve
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

const decisionNoteDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('decision-note-dialog')

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const isManager = computed(() => {
  return hasDid.value && (authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false)
})

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

watch(hasDid, (hasDid) => {
  templateEditorUiStore.reset()
  if (!hasDid) return

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
    const decisionNoteResult = await decisionNoteDialog.value?.reveal({ message: 'Add decision note?' })
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
    router.push({ name: ROUTES.TEMPLATES.LIST })
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
    const decisionNoteResult = await decisionNoteDialog.value?.reveal({ message: 'Add decision note?' })
    if (decisionNoteResult?.isCanceled) {
      return
    } else if (decisionNoteResult?.data) {
      decisionNote.value = decisionNoteResult.data
    }
    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: decisionNote.value ? [decisionNote.value] : []
    })
    router.push({ name: ROUTES.TEMPLATES.LIST })
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
  const decisionNoteResult = await decisionNoteDialog.value?.reveal({ message: 'Add reason:', requiredText: true })
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
    router.push({ name: ROUTES.TEMPLATES.LIST })
  } catch (error) {
    console.error('Rejection failed', error)
  } finally {
    isSubmitting.value = false
  }
}
</script>
