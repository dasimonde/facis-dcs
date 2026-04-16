<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">
    <TemplateEditors title="View Template" />

    <!-- Pinned Footer -->
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="router.back()">Back</button>
        <SubmitSelectionDialog
          v-if="isCreator && state === TemplateState.draft"
          dialog-type="template"
          @submit="submitTemplate"
          class="btn btn-primary flex-1"
        />
        <button v-if="isCreator && state === TemplateState.rejected" class="btn btn-primary flex-1" @click="submitRejectedTemplate">Submit</button>
        <TemplateManagerActions v-if="contractTemplate && isManager" :item="contractTemplate" class="btn btn-primary flex-1" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import TemplateManagerActions from '@/components/lists/template/TemplateManagerActions.vue'
import SubmitSelectionDialog from '@/components/SubmitSelectionDialog.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import type { SelectedUserRole } from '@/models/user'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState } from '@/types/contract-template-state'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { storeToRefs } from 'pinia'
import { computed, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const authStore = useAuthStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()
const { state } = storeToRefs(draftStore)

const hasDid = computed(() => !!route.params.did)
const hasChosenType = ref(false)

const isCreator = computed(() => {
  return draftStore.created_by === authStore.user?.username
})

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
        created_by: template.created_by
      })
    })
    .catch(error => {
      console.error('Failed to load template for editing', error)
    })

}, { immediate: true })

const submitTemplate = async (result: SelectedUserRole[]) => {
  try {
    if (!draftStore.did || !draftStore.updated_at) return
    const reviewers = result.filter((user) => user.role === 'TEMPLATE_REVIEWER').map((user) => user.user.username)
    const approver = result.find((user) => user.role === 'TEMPLATE_APPROVER')?.user.username!
    const response = await contractTemplateService.submit({
      did: draftStore.did,
      updated_at: draftStore.updated_at,
      reviewers: reviewers,
      approver: approver,
    })
    if (response?.did) {
      router.push({ name: ROUTES.TEMPLATES.LIST })
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
      updated_at: draftStore.updated_at
    })
    if (response.did) {
      router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (error) {
    console.error('Template Submission failed', error)
  }
}
</script>
