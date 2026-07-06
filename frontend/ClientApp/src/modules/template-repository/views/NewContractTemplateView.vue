<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { TemplateState } from '@/types/contract-template-state'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import TemplateTypeSelect from '@template-repository/components/TemplateTypeSelect.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()
const { templateType } = storeToRefs(draftStore)

const isEditMode = computed(() => !!route.params.did)
const hasChosenType = ref(false)
const showTypeSelectionOnly = computed(() => !isEditMode.value && !hasChosenType.value)
const title = computed(() => (isEditMode.value ? 'Update Template' : 'Create Template'))

const { isManager } = useTemplatePermissions()

function onTemplateTypeChosen(value: typeof templateType.value) {
  draftStore.reset({ templateType: value })
  hasChosenType.value = true
}

watch(
  isEditMode,
  (isEdit) => {
    templateEditorUiStore.reset()
    if (isEdit) {
      hasChosenType.value = true
      // load template data into draftStore
      const did = Array.isArray(route.params.did) ? route.params.did[0] : route.params.did
      if (!did) return
      contractTemplateService
        .retrieveById({ did })
        .then((template) => {
          if (!template) {
            draftStore.reset()
            return
          }
          const uneditableStates = [
            TemplateState.deprecated,
            TemplateState.registered,
            TemplateState.published,
            TemplateState.registered,
          ].map((s) => s.toLowerCase())
          templateEditorUiStore.setTemplateEditable(!uneditableStates.includes(template.state.toLowerCase()))

          draftStore.loadDocument(template.template_data, {
            did: template.did,
            name: template.name ?? '',
            description: template.description ?? '',
            templateType: template.template_type,
            state: template.state,
            version: template.version ?? null,
            document_number: template.document_number ?? null,
            updated_at: template.updated_at ?? null,
            responsible: template.responsible ?? null,
          })
        })
        .catch((error: unknown) => {
          console.error('Failed to load template for editing', error)
        })
    } else {
      draftStore.reset()
      templateEditorUiStore.setTemplateEditable(true)
      hasChosenType.value = false
    }
  },
  { immediate: true },
)

const isSubmitting = ref(false)
const submitError = ref<string | null>(null)

const submit = async () => {
  isSubmitting.value = true
  submitError.value = null
  try {
    if (!draftStore.hasTemplateId) {
      // create a draft template
      const data = draftStore.templateCreateRequestData
      await contractTemplateService.create(data)
    } else {
      if (isManager.value) {
        // update existing template
        const data = draftStore.templateUpdateManageRequestData
        if (data) {
          await contractTemplateService.updateManage(data)
        }
      } else {
        // update existing template
        const data = draftStore.templateUpdateRequestData
        if (data) {
          await contractTemplateService.update(data)
        }
      }
    }
    await router.push({ name: ROUTES.TEMPLATES.LIST })
  } catch (error) {
    console.error('Submission failed', error)
    submitError.value = error instanceof Error ? error.message : String(error)
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <!-- Create flow: show only type selection until user chooses -->
    <div v-if="showTypeSelectionOnly" class="mx-auto flex max-w-4xl flex-col gap-6 px-6 py-12">
      <h1 class="text-2xl font-bold text-base-content">Choose contract type</h1>
      <TemplateTypeSelect :model-value="templateType" @update:model-value="onTemplateTypeChosen($event)" />
      <div class="flex justify-end pt-4">
        <button type="button" class="btn btn-outline" @click="router.back()">Back</button>
      </div>
    </div>
    <template v-else>
      <TemplateEditors :title="title" />

      <!-- Pinned Footer -->
      <div
        v-if="templateEditorUiStore.isTemplateEditable"
        class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100"
      >
        <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
          <button class="btn btn-outline md:w-32" @click="router.back()">Cancel</button>
          <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="submit">
            <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
            {{ isEditMode ? 'Update' : 'Create' }}
          </button>
        </div>
        <div v-if="submitError" class="mx-auto max-w-4xl px-6 pb-3">
          <p class="text-sm text-error">Save failed: {{ submitError }}</p>
        </div>
      </div>
    </template>
  </div>
</template>
