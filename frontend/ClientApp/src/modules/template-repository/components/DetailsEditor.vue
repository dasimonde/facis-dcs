<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useId } from 'vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { TemplateType } from '@template-repository/models/contract-template'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import {
  type ContractTemplate,
  requirableTemplates,
  type TemplateLevel,
  templateLevels,
} from '@/models/contract-template/contract-template'
import { contractTemplateService } from '@/services/contract-template-service'
import { type PartyType, partyTypes } from '@/types/party-type'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templateType, version } = storeToRefs(store)

const partyType = computed({
  get: () => store.partyType ?? '',
  set: (value: PartyType | '') => {
    store.partyType = value || undefined
  },
})

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim()),
})
const nameId = useId()

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value),
})
const descriptionId = useId()
const jurisdictionId = useId()
const industryId = useId()
const levelId = useId()

// DCS-FR-TR-02, DCS-FR-TR-23: the level and the templates a parent contract must come from.
const levelLabels: Record<TemplateLevel, string> = {
  FRAME_AGREEMENT: 'Frame agreement',
  AGREEMENT: 'Agreement',
  SUB_AGREEMENT: 'Sub-agreement',
  ADDENDUM: 'Addendum',
}
const { hasTemplateRole } = useTemplatePermissions()
const allTemplates = ref<ContractTemplate[]>([])
onMounted(async () => {
  if (!hasTemplateRole.value) return
  allTemplates.value = (await contractTemplateService.retrieve()).contract_templates
})
const candidates = computed(() => requirableTemplates(allTemplates.value, store.templateLevel, store.did))
const templateLevel = computed({
  get: () => store.templateLevel,
  set: (value: TemplateLevel) => {
    store.templateLevel = value
    store.requiredTemplates = store.requiredTemplates.filter((did) => candidates.value.some((t) => t.did === did))
  },
})
const templateName = (did: string) => allTemplates.value.find((t) => t.did === did)?.name ?? did
const props = defineProps<{
  partyTypeError?: string
  nameError?: string
  descriptionError?: string
}>()
const partyTypeId = useId()
const partyTypeErrorId = useId()
const nameErrorId = useId()
const descriptionErrorId = useId()
</script>

<template>
  <div class="grid grid-cols-1 gap-4">
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
            <p class="text-xs font-normal text-base-content/70">Top-level contract template that can serve as parent</p>
          </div>
        </div>
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.component ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Component</span>
            <p class="text-xs font-normal text-base-content/70">
              Reusable partial contract, embeddable in other templates
            </p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <label :for="partyTypeId" class="fieldset-legend">Party type</label>
      <select
        :id="partyTypeId"
        v-model="partyType"
        data-testid="template-party-type"
        class="select w-full"
        required
        :disabled="!uiStore.isTemplateEditable"
        :aria-invalid="!!props.partyTypeError"
        :aria-describedby="props.partyTypeError ? partyTypeErrorId : undefined"
      >
        <option value="" disabled>Select party type</option>
        <option v-for="option in partyTypes" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <p v-if="props.partyTypeError" :id="partyTypeErrorId" class="text-sm text-error">{{ props.partyTypeError }}</p>
    </fieldset>
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Global Name</legend>
      <label :for="nameId" class="sr-only">Global Name</label>
      <input
        :id="nameId"
        v-model="name"
        class="input-bordered input w-full"
        data-testid="template-global-name"
        type="text"
        required
        :aria-invalid="!!props.nameError"
        :aria-describedby="props.nameError ? nameErrorId : undefined"
        :disabled="!uiStore.isTemplateEditable"
      />
      <p v-if="props.nameError" :id="nameErrorId" class="text-sm text-error">{{ props.nameError }}</p>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Base Description</legend>
      <label :for="descriptionId" class="sr-only">Base Description</label>
      <textarea
        :id="descriptionId"
        v-model="description"
        class="textarea-bordered textarea h-24 w-full"
        data-testid="template-base-description"
        required
        :aria-invalid="!!props.descriptionError"
        :aria-describedby="props.descriptionError ? descriptionErrorId : undefined"
        :disabled="!uiStore.isTemplateEditable"
      ></textarea>
      <p v-if="props.descriptionError" :id="descriptionErrorId" class="text-sm text-error">
        {{ props.descriptionError }}
      </p>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <label :for="jurisdictionId" class="fieldset-legend">Jurisdiction</label>
      <input
        :id="jurisdictionId"
        v-model.trim="store.jurisdiction"
        class="input-bordered input w-full"
        data-testid="template-jurisdiction"
        type="text"
        :disabled="!uiStore.isTemplateEditable"
      />
    </fieldset>
    <fieldset class="fieldset border-none p-0">
      <label :for="industryId" class="fieldset-legend">Industry</label>
      <input
        :id="industryId"
        v-model.trim="store.industry"
        class="input-bordered input w-full"
        data-testid="template-industry"
        type="text"
        :disabled="!uiStore.isTemplateEditable"
      />
    </fieldset>

    <fieldset v-if="templateType === TemplateType.contractTemplate" class="fieldset border-none p-0">
      <label :for="levelId" class="fieldset-legend">Hierarchy level</label>
      <select
        :id="levelId"
        v-model="templateLevel"
        data-testid="template-level"
        class="select w-full"
        :disabled="!uiStore.isTemplateEditable"
      >
        <option v-for="level in templateLevels" :key="level" :value="level">{{ levelLabels[level] }}</option>
      </select>
    </fieldset>
    <fieldset
      v-if="templateType === TemplateType.contractTemplate"
      class="fieldset border-none p-0"
      data-testid="template-required-templates"
    >
      <legend class="fieldset-legend">Parent contract must come from</legend>
      <template v-if="uiStore.isTemplateEditable">
        <p v-if="!candidates.length" class="text-xs text-base-content/70">
          No approved template of a higher level is available.
        </p>
        <label v-for="candidate in candidates" :key="candidate.did" class="label cursor-pointer justify-start gap-2">
          <input
            v-model="store.requiredTemplates"
            type="checkbox"
            class="checkbox checkbox-sm"
            :value="candidate.did"
          />
          <span class="text-sm">
            {{ candidate.name ?? candidate.did }} (v{{ candidate.version }},
            {{ levelLabels[candidate.template_level ?? 'AGREEMENT'] }})
          </span>
        </label>
      </template>
      <ul v-else class="text-sm">
        <li v-for="did in store.requiredTemplates" :key="did">{{ templateName(did) }}</li>
        <li v-if="!store.requiredTemplates.length" class="text-base-content/70">No required template</li>
      </ul>
    </fieldset>
  </div>
</template>
