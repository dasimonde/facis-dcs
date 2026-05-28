<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <TemplateEditors title="View Template" />

    <!-- Pinned Footer -->
    <div v-if="$route.params.did === did" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <CopyTemplateButton v-if="isCreator || isManager" class="btn flex-1 btn-primary" />
        <template v-if="isCreator">
          <SubmitSelectionDialog
            v-if="state === TemplateState.draft"
            dialog-type="template"
            class="btn flex-1 btn-primary"
            @submit="submitTemplate"
          />
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

<script setup lang="ts">
import SubmitSelectionDialog from '@/components/SubmitSelectionDialog.vue'
import TemplateManagerActions from '@/components/template/TemplateManagerActions.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import type { SelectedUserRole } from '@/models/user'
import { contractTemplateService } from '@/services/contract-template-service'
import { useNavStore } from '@/stores/nav-store'
import { TemplateState } from '@/types/contract-template-state'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { storeToRefs } from 'pinia'
import { ref, watch, type Ref } from 'vue'
import CopyTemplateButton from '../components/CopyTemplateButton.vue'

const props = defineProps<{
  did: string
}>()

const navStore = useNavStore()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()
const { state } = storeToRefs(draftStore)

const hasChosenType = ref(false)

const { isCreator, isManager } = useTemplatePermissions()

const contractTemplate: Ref<PartialContractTemplate | null> = ref(null)

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
          created_by: template.created_by,
          responsible_persons: template.responsible_persons ?? null,
        })
      })
      .catch((error: unknown) => {
        console.error('Failed to load template for editing', error)
      })
  },
  { immediate: true },
)

const submitTemplate = async (result: SelectedUserRole[]) => {
  try {
    if (!draftStore.did || !draftStore.updated_at) return
    const reviewers = result.filter((user) => user.role === 'TEMPLATE_REVIEWER').map((user) => user.user.username)
    const approver = result.find((user) => user.role === 'TEMPLATE_APPROVER')!.user.username
    const response = await contractTemplateService.submit({
      did: draftStore.did,
      updated_at: draftStore.updated_at,
      reviewers: reviewers,
      approver: approver,
    })
    if (response?.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    console.error('Template Submission failed', error)
  }
}

const submitRejectedTemplate = async () => {
  try {
    if (!draftStore.did || !draftStore.updated_at) return
    const response = await contractTemplateService.submit({
      did: draftStore.did,
      updated_at: draftStore.updated_at,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    console.error('Template Submission failed', error)
  }
}
</script>
