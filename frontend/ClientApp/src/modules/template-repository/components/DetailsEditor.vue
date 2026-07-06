<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { TemplateType } from '@/modules/template-repository/models/contract-template'
import { contractTemplateService } from '@/services/contract-template-service'
import { useTemplateList } from '@/views/contract-template-list/ContractTemplateListController'
import { TemplateState } from '@/types/contract-template-state'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { useTemplatePermissions } from '../composables/useTemplatePermissions'

interface ComponentTemplateKey {
  did: string
  version: number
  document_number?: string
}

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templates: allTemplates } = useTemplateList()
const { templateType, blocks, subTemplateSnapshots, state, version } = storeToRefs(store)

const { isManager } = useTemplatePermissions()

const document_number = computed({
  get: () => store.document_number,
  set: (value: string) => store.updateDocumentNumber(value),
})

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim()),
})

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value),
})

const selectedComponents = computed<ComponentTemplateKey[]>(() =>
  subTemplateSnapshots.value.map((item) => ({
    did: item.did,
    version: item.version,
    document_number: item.document_number,
  })),
)
const showComponentPicker = ref(false)
const componentSearchQuery = ref('')

const isSameTemplate = (a: ComponentTemplateKey, b: ComponentTemplateKey) =>
  a.did === b.did && a.version === b.version && a.document_number === b.document_number
const isSelected = (t: ComponentTemplateKey) => selectedComponents.value.some((s) => isSameTemplate(s, t))

const filteredComponentTemplates = computed(() => {
  const q = componentSearchQuery.value.toLowerCase()
  const selectableStates = new Set<string>([TemplateState.approved, TemplateState.published])
  return allTemplates.value.filter(
    (t) =>
      !isSelected(t) &&
      selectableStates.has(t.state) &&
      t.template_type === TemplateType.component &&
      (q === '' || (t.name ?? '').toLowerCase().includes(q) || t.did.toLowerCase().includes(q)),
  )
})

const getComponentTemplateName = (item: ComponentTemplateKey) =>
  subTemplateSnapshots.value.find((t) => isSameTemplate(t, item))?.name ??
  allTemplates.value.find((t) => isSameTemplate(t, item))?.name ??
  item.did

const addComponentTemplate = async (template: { did: string; version: number; document_number?: string }) => {
  if (isSelected(template)) return
  await contractTemplateService.retrieveById(template).then((fullTemplate) => {
    if (fullTemplate) store.addSubTemplateSnapshot(fullTemplate)
  })
  componentSearchQuery.value = ''
}

const isComponentReferenced = (item: ComponentTemplateKey): boolean => {
  const inOutline = store.blockIdsInOutline
  return blocks.value.some(
    (b) => b['@type'] === 'dcs:ApprovedTemplate' && inOutline.has(b['@id']) && b['dcs:templateDid'] === item.did,
  )
}

const removeComponentTemplate = (item: ComponentTemplateKey) => {
  if (isComponentReferenced(item)) return
  store.removeSubTemplateSnapshot(item)
}
</script>

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
          :class="templateType === TemplateType.contractTemplate ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Contract</span>
            <p class="text-xs font-normal text-base-content/60">Top-level contract template that can serve as parent</p>
          </div>
        </div>
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.component ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Component</span>
            <p class="text-xs font-normal text-base-content/60">
              Reusable partial contract, embeddable in other templates
            </p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset v-if="isManager" class="fieldset border-none p-0">
      <legend class="fieldset-legend">Template State</legend>
      <select
        v-model="state"
        class="input-bordered select w-full"
        type="text"
        required
        :disabled="!uiStore.isTemplateEditable"
      >
        <option>DRAFT</option>
        <option>REJECTED</option>
        <option>SUBMITTED</option>
        <option>REVIEWED</option>
        <option>APPROVED</option>
        <option>DELETED</option>
        <option v-if="state == TemplateState.registered">REGISTERED</option>
        <option v-if="state == TemplateState.published">PUBLISHED</option>
      </select>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Document number</legend>
      <input
        v-model="document_number"
        class="input-bordered input w-full"
        type="text"
        required
        :disabled="!uiStore.isTemplateEditable"
      />
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

    <!-- Component templates (only for Contract type) -->
    <fieldset v-if="templateType === TemplateType.contractTemplate" class="fieldset border-none p-0">
      <legend
        class="fieldset-legend inline-flex cursor-pointer items-center gap-1.5 select-none"
        @click="showComponentPicker = !showComponentPicker"
      >
        Component Templates
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-3 w-3 opacity-60 transition-transform duration-200"
          :class="{ 'rotate-180': showComponentPicker }"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </legend>

      <!-- Collapsible picker -->
      <div v-show="showComponentPicker" class="mt-1">
        <input
          v-model="componentSearchQuery"
          class="input-bordered input input-sm w-full"
          placeholder="Search templates…"
        />

        <ul class="menu mt-1 max-h-48 w-full flex-nowrap overflow-y-auto menu-sm rounded-box bg-base-200">
          <li v-if="!filteredComponentTemplates.length">
            <span class="pointer-events-none text-xs text-base-content/40 italic">
              {{ componentSearchQuery ? 'No results' : 'All component templates already added' }}
            </span>
          </li>
          <li v-for="t in filteredComponentTemplates" :key="`${t.did}-${t.version}-${t.document_number}`">
            <button type="button" class="group flex flex-col items-start gap-0" @click="addComponentTemplate(t)">
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
      <div v-if="selectedComponents.length" class="mt-3 flex flex-wrap gap-2">
        <div
          v-for="item in selectedComponents"
          :key="`${item.did}-${item.version}-${item.document_number}`"
          class="badge gap-1 badge-outline py-3 badge-primary"
        >
          <span>{{ getComponentTemplateName(item) }}</span>
          <button
            type="button"
            :disabled="isComponentReferenced(item) || !uiStore.isTemplateEditable"
            :title="isComponentReferenced(item) ? 'Cannot remove: used in document' : undefined"
            class="text-error transition-opacity hover:opacity-70 disabled:cursor-not-allowed disabled:opacity-40"
            @click="removeComponentTemplate(item)"
          >
            ✕
          </button>
        </div>
      </div>
      <p v-else class="mt-2 fieldset-label">No component templates selected yet.</p>
    </fieldset>
  </div>
</template>
