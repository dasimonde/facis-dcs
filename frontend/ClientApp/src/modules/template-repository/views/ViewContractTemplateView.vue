<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, type Ref, ref, watch } from 'vue'
import WorkflowStageBanner from '@core/components/WorkflowStageBanner.vue'
import { templateStory, toBannerActions } from '@core/workflow-story'
import CopyTemplateButton from '@template-repository/components/CopyTemplateButton.vue'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import TemplateManagerActions from '@/components/template/TemplateManagerActions.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractTemplateService } from '@/services/contract-template-service'
import { useNavStore } from '@/stores/nav-store'
import { TemplateState } from '@/types/contract-template-state'
import { reportActionError } from '@/utils/report-action-error'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

const props = defineProps<{
  did: string
  embedded?: boolean
}>()

const navStore = useNavStore()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useDcsDraftStore()
const { state, templateType } = storeToRefs(draftStore)

const hasChosenType = ref(false)

const { isCreator, isManager } = useTemplatePermissions()

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

const story = computed(() => templateStory(state.value, { templateType: templateType.value }))

watch(
  () => props.did,
  (newDid, oldDid) => {
    templateEditorUiStore.reset()
    if (newDid === oldDid) return

    hasChosenType.value = true
    const did = `${props.did}`
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
          created_by: template.created_by,
          responsible: template.responsible ?? null,
        })
      })
      .catch((error: unknown) => {
        reportActionError(error, 'Load template')
      })
  },
  { immediate: true },
)

const submitTemplate = async () => {
  try {
    if (!draftStore.did || !draftStore.updated_at) {
      reportActionError(new Error('The current template version is unavailable.'), 'Submit template')
      return
    }
    const response = await contractTemplateService.submit({
      did: draftStore.did,
      updated_at: draftStore.updated_at,
    })
    if (response?.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    reportActionError(error, 'Submit template')
  }
}

const submitRejectedTemplate = async () => {
  try {
    if (!draftStore.did || !draftStore.updated_at) {
      reportActionError(new Error('The current template version is unavailable.'), 'Resubmit template')
      return
    }
    const response = await contractTemplateService.submit({
      did: draftStore.did,
      updated_at: draftStore.updated_at,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    reportActionError(error, 'Resubmit template')
  }
}

const { download: downloadExport, exporting } = useDocumentExport()

const exportPDF = async () => {
  await downloadExport(() => contractTemplateService.exportPdf(props.did), `template-${props.did}.pdf`)
}
</script>

<template>
  <div :class="embedded ? 'flex flex-1 flex-col' : '-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8'">
    <TemplateEditors title="View Template">
      <template #before-tabs>
        <WorkflowStageBanner
          v-if="!embedded && state"
          :steps="story.steps"
          :current-key="story.currentKey"
          :headline="story.headline"
          :narrative="story.narrative"
          :actions="toBannerActions(story.actionHints)"
        />
        <slot name="before-tabs" />
      </template>
    </TemplateEditors>

    <!-- Pinned Footer -->
    <div v-if="$route.params.did === did" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <button class="btn btn-outline md:w-32" :disabled="exporting" @click="exportPDF">Export PDF</button>
        <CopyTemplateButton :disabled="!isCreator && !isManager" class="btn flex-1 btn-primary" />
        <template v-if="isCreator || isManager">
          <button v-if="state === TemplateState.draft" class="btn flex-1 btn-primary" @click="submitTemplate">
            Submit
          </button>
          <button
            v-if="state === TemplateState.rejected"
            class="btn flex-1 btn-primary"
            @click="submitRejectedTemplate"
          >
            Submit
          </button>
        </template>
        <TemplateManagerActions
          v-if="contractTemplate && isManager"
          :template="contractTemplate"
          class="btn flex-1 btn-primary"
        />
      </div>
    </div>
  </div>
</template>
