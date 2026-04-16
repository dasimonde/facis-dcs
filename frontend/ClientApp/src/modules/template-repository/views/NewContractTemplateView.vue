<template>
    <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">
        <!-- Create flow: show only type selection until user chooses -->
        <div v-if="showTypeSelectionOnly" class="max-w-4xl mx-auto px-6 py-12 flex flex-col gap-6">
            <h1 class="text-2xl font-bold text-base-content">Choose contract type</h1>
            <TemplateTypeSelect :model-value="templateType" @update:model-value="onTemplateTypeChosen($event)" />
            <div class="flex justify-end pt-4">
                <button type="button" class="btn btn-ghost" @click="router.back()">Cancel</button>
            </div>
        </div>
        <template v-else>
            <TemplateEditors :title="title" />

            <!-- Pinned Footer -->
            <div v-if="templateEditorUiStore.isTemplateEditable" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
                <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
                    <button class="btn btn-ghost md:w-32" @click="router.back()">Cancel</button>
                    <button @click="submit" class="btn btn-primary flex-1" :disabled="isSubmitting">
                        <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
                        {{ isEditMode ? 'Update Template' : 'Create' }}
                    </button>
                </div>
            </div>
        </template>
    </div>
</template>

<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { TemplateState } from '@/types/contract-template-state'
import TemplateEditors from '@template-repository/components/TemplateEditors.vue'
import TemplateTypeSelect from '@template-repository/components/TemplateTypeSelect.vue'
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
const title = computed(() => isEditMode.value ? 'Update Template' : 'Create Template')

function onTemplateTypeChosen(value: typeof templateType.value) {
    draftStore.reset({ templateType: value })
    hasChosenType.value = true
}

watch(isEditMode, (isEdit) => {
    templateEditorUiStore.reset()
    if (isEdit) {
        hasChosenType.value = true
        // load template data into draftStore
        const did = `${route.params.did}`
        contractTemplateService.retrieveById({ did })
            .then(async template => {
                if (!template) {
                    draftStore.reset()
                    return
                }
                const uneditableStates = [TemplateState.approved, TemplateState.deleted, TemplateState.deprecated, TemplateState.registered].map((s) => s.toLowerCase())
                templateEditorUiStore.setTemplateEditable(!(uneditableStates.includes(template.state.toLowerCase())))

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

    }
    else {
        draftStore.reset()
        templateEditorUiStore.setTemplateEditable(true)
        hasChosenType.value = false 
    }
}, { immediate: true })

const isSubmitting = ref(false)

const submit = async () => {
    isSubmitting.value = true
    try {
        if (!draftStore.hasTemplateId) {
            // create a draft template
            const data = draftStore.templateCreateRequestData
            await contractTemplateService.create(data)
        } else {
            // update existing template
            const data = draftStore.templateUpdateRequestData
            if (data) {
                await contractTemplateService.update(data)
            }
        }
        router.push({ name: ROUTES.TEMPLATES.LIST })
    } catch (error) {
        console.error('Submission failed', error)
    } finally {
        isSubmitting.value = false
    }
}
</script>
