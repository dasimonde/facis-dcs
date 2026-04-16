<template>
    <div class="grid grid-cols-1 gap-4">
        <!-- Contract Kind -->
        <fieldset class="fieldset p-0 border-none">
            <legend class="fieldset-legend">Contract Type</legend>
            <div class="grid grid-cols-2 gap-3 mt-1">
                <div class="card border-2 transition-all pointer-events-none"
                    :class="templateType === TemplateType.frameContract
                        ? 'border-primary bg-primary/5'
                        : 'border-base-300'">
                    <div class="card-body p-4 gap-1">
                        <span class="card-title text-sm">Frame Contract</span>
                        <p class="text-xs text-base-content/60 font-normal">Top-level agreement that groups subcontracts</p>
                    </div>
                </div>
                <div class="card border-2 transition-all pointer-events-none"
                    :class="templateType === TemplateType.subContract
                        ? 'border-primary bg-primary/5'
                        : 'border-base-300'">
                    <div class="card-body p-4 gap-1">
                        <span class="card-title text-sm">Subcontract</span>
                        <p class="text-xs text-base-content/60 font-normal">Scoped agreement under a frame contract</p>
                    </div>
                </div>
            </div>
        </fieldset>

        <fieldset class="fieldset p-0 border-none">
            <legend class="fieldset-legend">Global Name</legend>
            <input v-model="name" class="input input-bordered w-full" type="text" required :disabled="!uiStore.isTemplateEditable"/>
        </fieldset>

        <fieldset class="fieldset p-0 border-none">
            <legend class="fieldset-legend">Base Description</legend>
            <textarea v-model="description" class="textarea textarea-bordered w-full h-24" required :disabled="!uiStore.isTemplateEditable"></textarea>
        </fieldset>

        <!-- Subcontracts (only for frame contracts) -->
        <fieldset v-if="templateType === TemplateType.frameContract" class="fieldset p-0 border-none">
            <legend class="fieldset-legend cursor-pointer select-none inline-flex items-center gap-1.5"
                @click="showSubcontractPicker = !showSubcontractPicker">
                Subcontract Templates
                <svg xmlns="http://www.w3.org/2000/svg"
                    class="w-3 h-3 transition-transform duration-200 opacity-60"
                    :class="{ 'rotate-180': showSubcontractPicker }"
                    fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
            </legend>

            <!-- Collapsible picker -->
            <div v-show="showSubcontractPicker" class="mt-1">
                <input v-model="subcontractSearchQuery"
                    class="input input-bordered input-sm w-full"
                    placeholder="Search templates…" />

                <ul class="menu menu-sm w-full bg-base-200 rounded-box mt-1 max-h-48 overflow-y-auto flex-nowrap">
                    <li v-if="!filteredSubcontractTemplates.length">
                        <span class="text-base-content/40 italic text-xs pointer-events-none">
                            {{ subcontractSearchQuery ? 'No results' : 'All templates already selected' }}
                        </span>
                    </li>
                    <li v-for="t in filteredSubcontractTemplates" :key="`${t.did}-${t.version}-${t.document_number}`">
                        <button type="button" @click="addSubcontractTemplate(t)"
                            class="group flex flex-col items-start gap-0">
                            <span class="font-medium text-sm">{{ t.name }}</span>
                            <span class="text-xs text-base-content/50 italic overflow-hidden max-h-0 group-hover:max-h-12 transition-all duration-200 ease-in-out">
                                {{ t.description }}
                            </span>
                        </button>
                    </li>
                </ul>
            </div>

            <!-- Selected templates (always visible) -->
            <div v-if="selectedSubcontracts.length" class="flex flex-wrap gap-2 mt-3">
                <div v-for="item in selectedSubcontracts" :key="`${item.did}-${item.version}-${item.document_number}`"
                    class="badge badge-primary badge-outline gap-1 py-3">
                    <span>{{ getSubcontractTemplateName(item) }}</span>
                    <button type="button" @click="removeSubcontractTemplate(item)"
                        :disabled="isSubcontractReferenced(item)"
                        :title="isSubcontractReferenced(item) ? 'Cannot remove: used in document' : undefined"
                        class="text-error hover:opacity-70 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed">✕</button>
                </div>
            </div>
            <p v-else class="fieldset-label mt-2">No subcontract templates selected yet.</p>
        </fieldset>
    </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { TemplateType, isApprovedTemplateBlock } from '@template-repository/models/contract-templace'
import { contractTemplateService } from '@/services/contract-template-service'
import { useTemplateTable } from '@/views/contract-template-list/ContractTemplateListController'
import { TemplateState } from '@/types/contract-template-state'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

interface SubcontractKey {
    did: string
    version?: number
    document_number?: string
}

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templates: allTemplates } = useTemplateTable()
const { templateType, documentBlocks, subTemplateSnapshots } = storeToRefs(store)

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim())
})

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value)
})

const selectedSubcontracts = computed<SubcontractKey[]>(() =>
    subTemplateSnapshots.value.map((item) => ({
        did: item.did,
        version: item.version,
        document_number: item.document_number,
    }))
)
const showSubcontractPicker = ref(false)
const subcontractSearchQuery = ref('')

const isSameTemplate = (a: SubcontractKey, b: SubcontractKey) => a.did === b.did && a.version === b.version && a.document_number === b.document_number
const isSelected = (t: SubcontractKey) =>
    selectedSubcontracts.value.some(s => isSameTemplate(s, t))

const filteredSubcontractTemplates = computed(() => {
    const q = subcontractSearchQuery.value.toLowerCase()
    const selectableStates = new Set<string>([TemplateState.approved, TemplateState.registered])
    return allTemplates.value.filter(t =>
        !isSelected(t) && selectableStates.has(t.state) && t.template_type === TemplateType.subContract &&
        (q === '' || (t.name ?? '').toLowerCase().includes(q) || t.did.toLowerCase().includes(q))
    )
})

const getSubcontractTemplateName = (item: SubcontractKey) =>
    subTemplateSnapshots.value.find(t => isSameTemplate(t, item))?.name ??
    allTemplates.value.find(t => isSameTemplate(t, item))?.name ??
    item.did

const addSubcontractTemplate = async (template: { did: string; version?: number; document_number?: string }) => {
    if (isSelected(template)) return
    await contractTemplateService.retrieveById(template).then(fullTemplate => {
        if (fullTemplate) store.addSubTemplateSnapshot(fullTemplate)
    })
    subcontractSearchQuery.value = ''
}

const isSubcontractReferenced = (item: SubcontractKey): boolean => {
    const inOutline = store.blockIdsInOutline
    return documentBlocks.value.some(
        b => isApprovedTemplateBlock(b) && inOutline.has(b.blockId) && b.templateId === item.did
    )
}

const removeSubcontractTemplate = (item: SubcontractKey) => {
    if (isSubcontractReferenced(item)) return
    store.removeSubTemplateSnapshot(item)
}

</script>
