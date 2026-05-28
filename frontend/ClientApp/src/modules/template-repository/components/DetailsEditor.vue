<template>
  <div class="grid grid-cols-1 gap-4">
    <!-- Contract Kind -->
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Version: {{ version }}</legend>
    </fieldset>
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Contract Type</legend>
      <div class="mt-1 grid grid-cols-2 gap-3">
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.frameContract ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Frame Contract</span>
            <p class="text-xs font-normal text-base-content/60">Top-level agreement that groups subcontracts</p>
          </div>
        </div>
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.subContract ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Subcontract</span>
            <p class="text-xs font-normal text-base-content/60">Scoped agreement under a frame contract</p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Version {{ version }}</legend>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Global Name</legend>
      <input
        v-model="name"
        class="input-bordered input w-full"
        type="text"
        required
        :disabled="!uiStore.isTemplateEditable"
      />
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Base Description</legend>
      <textarea
        v-model="description"
        class="textarea-bordered textarea h-24 w-full"
        required
        :disabled="!uiStore.isTemplateEditable"
      ></textarea>
    </fieldset>

    <!-- Subcontracts (only for frame contracts) -->
    <fieldset v-if="templateType === TemplateType.frameContract" class="fieldset border-none p-0">
      <legend
        class="fieldset-legend inline-flex cursor-pointer items-center gap-1.5 select-none"
        @click="showSubcontractPicker = !showSubcontractPicker"
      >
        Subcontract Templates
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-3 w-3 opacity-60 transition-transform duration-200"
          :class="{ 'rotate-180': showSubcontractPicker }"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </legend>

      <!-- Collapsible picker -->
      <div v-show="showSubcontractPicker" class="mt-1">
        <input
          v-model="subcontractSearchQuery"
          class="input-bordered input input-sm w-full"
          placeholder="Search templates…"
        />

        <ul class="menu mt-1 max-h-48 w-full flex-nowrap overflow-y-auto menu-sm rounded-box bg-base-200">
          <li v-if="!filteredSubcontractTemplates.length">
            <span class="pointer-events-none text-xs text-base-content/40 italic">
              {{ subcontractSearchQuery ? 'No results' : 'All templates already selected' }}
            </span>
          </li>
          <li v-for="t in filteredSubcontractTemplates" :key="`${t.did}-${t.version}-${t.document_number}`">
            <button type="button" class="group flex flex-col items-start gap-0" @click="addSubcontractTemplate(t)">
              <span class="text-sm font-medium">{{ t.name }}</span>
              <span
                class="max-h-0 overflow-hidden text-xs text-base-content/50 italic transition-all duration-200 ease-in-out group-hover:max-h-12"
              >
                {{ t.description }}
              </span>
            </button>
          </li>
        </ul>
      </div>

      <!-- Selected templates (always visible) -->
      <div v-if="selectedSubcontracts.length" class="mt-3 flex flex-wrap gap-2">
        <div
          v-for="item in selectedSubcontracts"
          :key="`${item.did}-${item.version}-${item.document_number}`"
          class="badge gap-1 badge-outline py-3 badge-primary"
        >
          <span>{{ getSubcontractTemplateName(item) }}</span>
          <button
            type="button"
            :disabled="isSubcontractReferenced(item) || !uiStore.isTemplateEditable"
            :title="isSubcontractReferenced(item) ? 'Cannot remove: used in document' : undefined"
            class="text-error transition-opacity hover:opacity-70 disabled:cursor-not-allowed disabled:opacity-40"
            @click="removeSubcontractTemplate(item)"
          >
            ✕
          </button>
        </div>
      </div>
      <p v-else class="mt-2 fieldset-label">No subcontract templates selected yet.</p>
    </fieldset>

    <fieldset v-if="state !== TemplateState.draft" class="fieldset border-none p-0">
      <div class="collapse-arrow collapse [&>input~.collapse-title::after]:scale-75">
        <input type="checkbox" name="responsibles" />
        <legend class="collapse-title fieldset-legend pl-0 font-semibold">Responsible Persons</legend>
        <div class="collapse-content grid px-0">
          <ul class="list col-start-1 row-start-1">
            <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Creator</li>
            <li class="list-row py-0">{{ responsible_persons?.creator }}</li>
          </ul>
          <ul class="list col-start-2 row-start-1">
            <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Approver</li>
            <li class="list-row py-0">{{ responsible_persons?.approver }}</li>
          </ul>
          <ul class="list col-start-1 row-start-2">
            <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Reviewers</li>
            <li v-for="(reviewer, i) in responsible_persons?.reviewers" :key="i + reviewer" class="list-row py-0">
              {{ reviewer }}
            </li>
          </ul>
        </div>
      </div>
    </fieldset>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { TemplateType, isApprovedTemplateBlock } from '@template-repository/models/contract-templace'
import { contractTemplateService } from '@/services/contract-template-service'
import { useTemplateList } from '@/views/contract-template-list/ContractTemplateListController'
import { TemplateState } from '@/types/contract-template-state'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

interface SubcontractKey {
  did: string
  version: number
  document_number?: string
}

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templates: allTemplates } = useTemplateList()
const { templateType, documentBlocks, subTemplateSnapshots, state, responsible_persons, version } = storeToRefs(store)

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim()),
})

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value),
})

const selectedSubcontracts = computed<SubcontractKey[]>(() =>
  subTemplateSnapshots.value.map((item) => ({
    did: item.did,
    version: item.version,
    document_number: item.document_number,
  })),
)
const showSubcontractPicker = ref(false)
const subcontractSearchQuery = ref('')

const isSameTemplate = (a: SubcontractKey, b: SubcontractKey) =>
  a.did === b.did && a.version === b.version && a.document_number === b.document_number
const isSelected = (t: SubcontractKey) => selectedSubcontracts.value.some((s) => isSameTemplate(s, t))

const filteredSubcontractTemplates = computed(() => {
  const q = subcontractSearchQuery.value.toLowerCase()
  const selectableStates = new Set<string>([TemplateState.approved, TemplateState.published])
  return allTemplates.value.filter(
    (t) =>
      !isSelected(t) &&
      selectableStates.has(t.state) &&
      t.template_type === TemplateType.subContract &&
      (q === '' || (t.name ?? '').toLowerCase().includes(q) || t.did.toLowerCase().includes(q)),
  )
})

const getSubcontractTemplateName = (item: SubcontractKey) =>
  subTemplateSnapshots.value.find((t) => isSameTemplate(t, item))?.name ??
  allTemplates.value.find((t) => isSameTemplate(t, item))?.name ??
  item.did

const addSubcontractTemplate = async (template: { did: string; version: number; document_number?: string }) => {
  if (isSelected(template)) return
  await contractTemplateService.retrieveById(template).then((fullTemplate) => {
    if (fullTemplate) store.addSubTemplateSnapshot(fullTemplate)
  })
  subcontractSearchQuery.value = ''
}

const isSubcontractReferenced = (item: SubcontractKey): boolean => {
  const inOutline = store.blockIdsInOutline
  return documentBlocks.value.some(
    (b) => isApprovedTemplateBlock(b) && inOutline.has(b.blockId) && b.templateId === item.did,
  )
}

const removeSubcontractTemplate = (item: SubcontractKey) => {
  if (isSubcontractReferenced(item)) return
  store.removeSubTemplateSnapshot(item)
}
</script>
