<template>
  <h3 class="mb-4 text-sm font-semibold text-base-content/80">{{ formTitle }}</h3>
  <div class="space-y-4">
    <div>
      <label class="label-text mb-1 block text-xs text-base-content/60">
        Rule name
        <RequiredIndicator />
      </label>
      <input
        v-model="newCondition.conditionName"
        type="text"
        class="input-bordered input input-sm w-full"
        :class="{ 'input-error': isRuleNameDuplicate }"
        placeholder=""
      />
      <p class="mt-0.5 text-xs text-base-content/50">Used when selecting this rule for a clause.</p>
      <p v-if="isRuleNameDuplicate" class="mt-0.5 text-xs text-error">Rule name already exists.</p>
    </div>

    <div class="space-y-4">
      <p class="label-text mb-1 text-xs text-base-content/60">Parameters</p>
      <div
        class="grid grid-cols-1 gap-x-3 rounded-lg border-2 border-dashed border-base-300 bg-base-200/50 p-3 md:grid-cols-12"
      >
        <p class="mb-2 text-xs font-medium text-base-content/70 md:col-span-12">New parameter</p>
        <div class="flex flex-col gap-1 md:col-span-4">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">
              Parameter name
              <RequiredIndicator />
            </span>
          </label>
          <input
            v-model="draftParameter.parameterName"
            type="text"
            class="input-bordered input input-sm h-9 w-full"
            :class="{ 'input-error': isParameterNameDuplicate }"
            placeholder="Label"
          />
          <p v-if="isParameterNameDuplicate" class="text-xs text-error">Parameter name already exists.</p>
        </div>
        <div class="flex flex-col gap-1 md:col-span-3">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">
              Type
              <RequiredIndicator />
            </span>
          </label>
          <select v-model="draftParameter.type" class="select-bordered select h-9 w-full select-sm">
            <option value="date">Date</option>
            <option value="string">Text</option>
            <option value="decimal">Decimal</option>
            <option value="integer">Integer</option>
          </select>
        </div>
        <div class="flex flex-col gap-1 md:col-span-2">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">Required</span>
          </label>
          <div class="flex h-9 items-center">
            <label class="label h-auto min-h-0 cursor-pointer justify-start gap-2 py-0">
              <input
                v-model="draftParameter.isRequired"
                type="checkbox"
                class="checkbox checkbox-sm checkbox-primary"
              />
              <span class="label-text text-xs">Required</span>
            </label>
          </div>
        </div>
        <div class="flex flex-col gap-1 md:col-span-2">
          <label class="invisible label min-h-0 py-0">
            <span class="label-text text-xs">Add</span>
          </label>
          <div class="flex h-9 items-center">
            <button
              type="button"
              class="btn w-full whitespace-nowrap btn-sm btn-secondary"
              :disabled="!canAddParameter"
              @click="addParameter"
            >
              + Add parameter
            </button>
          </div>
        </div>
      </div>

      <!-- Added parameters -->
      <div v-if="newCondition.parameters.length" class="space-y-2">
        <p class="text-xs font-medium text-base-content/70">Added parameters</p>
        <ul class="space-y-2">
          <li
            v-for="(param, idx) in newCondition.parameters"
            :key="idx"
            class="flex items-center gap-3 rounded-lg border border-base-300 bg-base-100 px-3 py-2.5"
          >
            <span class="rounded border border-base-300 bg-base-200/50 px-2 py-0.5 font-mono text-sm font-medium">
              {{ param.parameterName }}
            </span>
            <span class="badge badge-ghost badge-sm">{{ param.type }}</span>
            <span class="text-xs text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
            <button
              type="button"
              class="btn ml-auto shrink-0 text-error btn-ghost btn-xs"
              aria-label="Delete parameter"
              @click="deleteParameter(idx)"
            >
              ✕
            </button>
          </li>
        </ul>
      </div>
    </div>

    <div class="flex items-center justify-between">
      <button v-if="isEditMode" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">Cancel</button>
      <span v-else />
      <button type="button" class="btn btn-sm btn-secondary" :disabled="!canSubmitRule" @click="submitRule">
        {{ submitLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import {
  type SemanticCondition,
  type SemanticConditionParameter,
  SEMANTIC_CONDITION_SCHEMA_VERSION,
} from '@template-repository/models/contract-templace'

type NewConditionPayload = Omit<SemanticCondition, 'conditionId'>

const props = defineProps<{
  existingConditions: SemanticCondition[]
  mode?: 'create' | 'edit'
  initialCondition?: SemanticCondition | null
}>()

const emit = defineEmits<{
  'add-rule': [payload: NewConditionPayload]
  'update-rule': [payload: { conditionId: string; data: NewConditionPayload }]
  cancel: []
}>()

function defaultParam(): SemanticConditionParameter {
  return {
    parameterName: '',
    type: 'string',
    isRequired: true,
    operators: [],
    value: undefined,
  }
}

function getDefaultNewCondition(): NewConditionPayload {
  return {
    conditionName: '',
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    parameters: [],
  }
}

const newCondition = ref<NewConditionPayload>(getDefaultNewCondition())
const draftParameter = ref<SemanticConditionParameter>(defaultParam())
const isEditMode = computed(() => props.mode === 'edit')
const formTitle = computed(() => (isEditMode.value ? 'Edit rule' : 'New rule'))
const submitLabel = computed(() => (isEditMode.value ? 'Save changes' : 'Add rule'))

watch(
  () => [props.mode, props.initialCondition] as const,
  () => {
    if (!isEditMode.value || !props.initialCondition) {
      newCondition.value = getDefaultNewCondition()
      draftParameter.value = defaultParam()
      return
    }
    newCondition.value = {
      conditionName: props.initialCondition.conditionName,
      schemaVersion: props.initialCondition.schemaVersion,
      parameters: props.initialCondition.parameters.map((p) => ({ ...p })),
    }
    draftParameter.value = defaultParam()
  },
  { immediate: true },
)

const isParameterNameDuplicate = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
  const lower = name.toLowerCase()
  return newCondition.value.parameters.some((p) => p.parameterName.trim().toLowerCase() === lower)
})

const canAddParameter = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
  return !isParameterNameDuplicate.value
})

const isRuleNameDuplicate = computed(() => {
  const name = newCondition.value.conditionName?.trim()
  if (!name) return false
  const lower = name.toLowerCase()
  const currentConditionId = props.initialCondition?.conditionId
  return props.existingConditions.some(
    (c) =>
      // When in edit mode, the current condition is not included in the check
      c.conditionId !== currentConditionId && c.conditionName.trim().toLowerCase() === lower,
  )
})

const canSubmitRule = computed(() => {
  const name = newCondition.value.conditionName?.trim()
  if (!name) return false
  if (newCondition.value.parameters.length === 0) return false
  return !isRuleNameDuplicate.value
})

function addParameter() {
  if (!canAddParameter.value) return
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return
  newCondition.value.parameters.push({
    ...draftParameter.value,
    parameterName: name,
  })
  draftParameter.value = defaultParam()
}

function deleteParameter(index: number) {
  newCondition.value.parameters.splice(index, 1)
}

function buildConditionPayload(): NewConditionPayload {
  return {
    conditionName: newCondition.value.conditionName.trim(),
    schemaVersion: newCondition.value.schemaVersion,
    parameters: newCondition.value.parameters.map((p) => ({
      ...p,
      parameterName: p.parameterName.trim(),
    })),
  }
}

function submitRule() {
  if (!canSubmitRule.value) return
  const payload = buildConditionPayload()
  if (isEditMode.value) {
    if (!props.initialCondition?.conditionId) return
    emit('update-rule', { conditionId: props.initialCondition.conditionId, data: payload })
  } else {
    emit('add-rule', payload)
  }
  newCondition.value = getDefaultNewCondition()
  draftParameter.value = defaultParam()
}
</script>
