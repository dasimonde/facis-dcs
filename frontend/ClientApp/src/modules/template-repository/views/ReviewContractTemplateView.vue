<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, type Ref, ref, useTemplateRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import WorkflowStageBanner from '@core/components/WorkflowStageBanner.vue'
import { templateStory, toBannerActions } from '@core/workflow-story'
import CopyTemplateButton from '@template-repository/components/CopyTemplateButton.vue'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import TemplateManagerActions from '@/components/template/TemplateManagerActions.vue'
import VerificationFindingsDialog from '@/components/VerificationFindingsDialog.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractTemplateService } from '@/services/contract-template-service'
import { useNavStore } from '@/stores/nav-store'
import { reportActionError } from '@/utils/report-action-error'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

const router = useRouter()
const route = useRoute()
const navStore = useNavStore()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useDcsDraftStore()
const { state, templateType } = storeToRefs(draftStore)

const story = computed(() => templateStory(state.value, { templateType: templateType.value }))

const commentDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('comment-dialog')

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const { isCreator, isReviewer, isManager: isManagerBase } = useTemplatePermissions()
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

        draftStore.loadDocument(template.template_data, {
          did: template.did,
          name: template.name ?? '',
          description: template.description ?? '',
          templateType: template.template_type,
          state: template.state,
          version: template.version ?? null,
          updated_at: template.updated_at ?? null,
        })
      })
      .catch((error: unknown) => {
        reportActionError(error, 'Load template review')
      })
  },
  { immediate: true },
)

const isSubmitting = ref(false)
const forwardToApproval = async (comment: string) => {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    reportActionError(new Error('The current template version is unavailable.'), 'Forward template')
    return
  }
  try {
    isSubmitting.value = true

    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: comment ? [comment] : [],
      forward_to: 'APPROVAL',
      approver: '',
      reviewers: [],
    })
    await navStore.goToPreviousRoute()
  } catch (error) {
    reportActionError(error, 'Forward template')
  } finally {
    isSubmitting.value = false
  }
}

const returnToDraft = async () => {
  const did = draftStore.did
  const updatedAt = draftStore.updated_at
  if (!did || !updatedAt) {
    reportActionError(new Error('The current template version is unavailable.'), 'Return template to draft')
    return
  }
  try {
    let comment = ''
    const commentResult = await commentDialog.value?.reveal({
      message: 'Add comment?',
      editor: { requiredText: false },
    })
    if (commentResult?.isCanceled) {
      return
    } else if (commentResult?.data) {
      comment = commentResult.data
    }
    isSubmitting.value = true
    await contractTemplateService.submit({
      did,
      updated_at: updatedAt,
      comments: comment ? [comment] : [],
      forward_to: 'DRAFT',
      approver: '',
      reviewers: [],
    })
    await navStore.goToPreviousRoute()
  } catch (error) {
    reportActionError(error, 'Return template to draft')
  } finally {
    isSubmitting.value = false
  }
}

const { download: downloadExport, exporting } = useDocumentExport()

const exportPDF = async () => {
  const did = route.params?.did
  if (!did || Array.isArray(did)) return
  await downloadExport(() => contractTemplateService.exportPdf(did), `template-${did}.pdf`)
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <TemplateEditors title="Review Template">
      <template #before-tabs>
        <WorkflowStageBanner
          v-if="state"
          :steps="story.steps"
          :current-key="story.currentKey"
          :headline="story.headline"
          :narrative="story.narrative"
          :actions="toBannerActions(story.actionHints)"
        />
      </template>
    </TemplateEditors>

    <!-- Pinned Footer -->
    <div v-if="hasDid" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <!-- Comments container -->
      <ConfirmationModal ref="comment-dialog" />
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="router.back()">Back</button>
        <button class="btn btn-outline md:w-32" :disabled="exporting" @click="exportPDF">Export PDF</button>
        <CopyTemplateButton :disabled="!isCreator && !isManager" class="btn flex-1 btn-primary" />
        <!-- Return to draft / request changes -->
        <button
          class="btn flex-1 btn-primary"
          :disabled="(!isReviewer && !isManager) || isSubmitting"
          @click="returnToDraft"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Reject
        </button>
        <!-- Complete review (verify then forward to approval) -->
        <VerificationFindingsDialog
          class="btn flex-1 btn-primary"
          :disabled="(!isReviewer && !isManager) || isSubmitting"
          @confirm="forwardToApproval"
        />
        <TemplateManagerActions
          v-if="contractTemplate && isManager"
          :template="contractTemplate"
          class="btn flex-1 btn-primary"
        />
      </div>
    </div>
  </div>
</template>
