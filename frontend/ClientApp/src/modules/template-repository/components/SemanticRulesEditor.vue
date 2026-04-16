<template>
  <div class="space-y-6">
    <!-- Section 1: New rule -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="text-sm font-semibold text-base-content/80 mb-4">New rule</h3>
      <div class="space-y-4">
        <div>
          <label class="label-text text-xs text-base-content/60 block mb-1">Rule name
            <RequiredIndicator />
          </label>
          <input v-model="newCondition.conditionName" type="text" class="input input-bordered input-sm w-full"
            :class="{ 'input-error': isRuleNameDuplicate }" placeholder="" />
          <p class="text-xs text-base-content/50 mt-0.5">Used when selecting this rule for a clause.</p>
          <p v-if="isRuleNameDuplicate" class="text-xs text-error mt-0.5">Rule name already exists.</p>
        </div>

        <div class="space-y-4">
          <p class="label-text text-xs text-base-content/60 mb-1">Parameters</p>
          <div
            class="grid grid-cols-1 md:grid-cols-12 gap-x-3 p-3 rounded-lg border-2 border-dashed border-base-300 bg-base-200/50">
            <p class="md:col-span-12 text-xs font-medium text-base-content/70 mb-2">New parameter</p>
            <div class="md:col-span-4 flex flex-col gap-1">
              <label class="label py-0 min-h-0">
                <span class="label-text text-xs text-base-content/60">Parameter name
                  <RequiredIndicator />
                </span>
              </label>
              <input v-model="draftParameter.parameterName" type="text" class="input input-bordered input-sm w-full h-9"
                :class="{ 'input-error': isParameterNameDuplicate }" placeholder="Label" />
              <p v-if="isParameterNameDuplicate" class="text-xs text-error">Parameter name already exists.</p>
            </div>
            <div class="md:col-span-3 flex flex-col gap-1">
              <label class="label py-0 min-h-0">
                <span class="label-text text-xs text-base-content/60">Type
                  <RequiredIndicator />
                </span>
              </label>
              <select v-model="draftParameter.type" class="select select-bordered select-sm w-full h-9">
                <option value="date">Date</option>
                <option value="string">Text</option>
                <option value="decimal">Decimal</option>
                <option value="integer">Integer</option>
              </select>
            </div>
            <div class="md:col-span-2 flex flex-col gap-1">
              <label class="label py-0 min-h-0">
                <span class="label-text text-xs text-base-content/60">Required</span>
              </label>
              <div class="flex items-center h-9">
                <label class="label cursor-pointer justify-start gap-2 py-0 min-h-0 h-auto">
                  <input v-model="draftParameter.isRequired" type="checkbox"
                    class="checkbox checkbox-sm checkbox-primary" />
                  <span class="label-text text-xs">Required</span>
                </label>
              </div>
            </div>
            <div class="md:col-span-2 flex flex-col gap-1">
              <label class="label py-0 min-h-0 invisible">
                <span class="label-text text-xs">Add</span>
              </label>
              <div class="h-9 flex items-center">
                <button type="button" class="btn btn-secondary btn-sm w-full whitespace-nowrap"
                  :disabled="!canAddParameter" @click="addParameter">
                  + Add parameter
                </button>
              </div>
            </div>
          </div>

          <!-- Added parameters -->
          <div v-if="newCondition.parameters.length" class="space-y-2">
            <p class="text-xs font-medium text-base-content/70">Added parameters</p>
            <ul class="space-y-2">
              <li v-for="(param, idx) in newCondition.parameters" :key="idx"
                class="flex items-center gap-3 py-2.5 px-3 rounded-lg bg-base-100 border border-base-300">
                <span class="font-mono text-sm font-medium border border-base-300 rounded px-2 py-0.5 bg-base-200/50">{{
                  param.parameterName }}</span>
                <span class="badge badge-ghost badge-sm">{{ param.type }}</span>
                <span class="text-xs text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
                <button type="button" class="btn btn-ghost btn-xs text-error ml-auto shrink-0"
                  aria-label="Delete parameter" @click="deleteParameter(idx)"> ✕ </button>
              </li>
            </ul>
          </div>
        </div>

        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="!canAddRule" @click="addRule">
            Add rule
          </button>
        </div>
      </div>
    </section>

    <!-- Section 2: Existing rules -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="text-sm font-semibold text-base-content/80 mb-4">Existing rules</h3>
      <div class="space-y-2">
        <div v-for="condition in semanticConditions" :key="condition.conditionId"
          class="flex items-start gap-3 p-3 rounded-lg border border-base-300 bg-base-200/30 group hover:shadow-sm transition-all">
          <div class="flex-1 min-w-0">
            <div class="font-semibold text-sm text-base-content">
              {{ condition.conditionName }}
              <span class="font-normal text-base-content/60 ml-1">
                (used in {{ clauseCountByConditionId[condition.conditionId] ?? 0 }} clause{{
                  (clauseCountByConditionId[condition.conditionId] ?? 0) === 1 ? '' : 's' }})
              </span>
            </div>
            <div class="flex flex-wrap gap-2 mt-2">
              <div v-for="(p, i) in condition.parameters" :key="i" class="badge badge-ghost badge-sm gap-1">
                <span>{{ p.parameterName }}</span>
                <span class="opacity-70">({{ p.type }}, {{ p.isRequired ? 'required' : 'optional' }})</span>
              </div>
            </div>
          </div>
          <button v-if="uiStore.isTemplateEditable" type="button"
            class="btn btn-ghost btn-xs text-error opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
            aria-label="Delete rule" @click="deleteRule(condition.conditionId)">
            ✕
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import {
  type SemanticCondition,
  type SemanticConditionParameter,
  SEMANTIC_CONDITION_SCHEMA_VERSION,
  isClauseBlock,
} from '@template-repository/models/contract-templace'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { semanticConditions, documentBlocks } = storeToRefs(store)

/** Number of clause blocks that reference each conditionId. */
const clauseCountByConditionId = computed(() => {
  const counts: Record<string, number> = {}
  for (const block of documentBlocks.value) {
    if (!isClauseBlock(block)) continue
    for (const id of block.conditionIds) {
      counts[id] = (counts[id] ?? 0) + 1
    }
  }
  return counts
})

type NewConditionPayload = Omit<SemanticCondition, 'conditionId'>

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

const isParameterNameDuplicate = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
  const lower = name.toLowerCase()
  return newCondition.value.parameters.some(
    (p) => p.parameterName.trim().toLowerCase() === lower
  )
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
  return semanticConditions.value.some(
    (c) => c.conditionName.trim().toLowerCase() === lower
  )
})

const canAddRule = computed(() => {
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

function addRule() {
  if (!canAddRule.value) return
  const payload = buildConditionPayload()
  store.addSemanticCondition(payload)
  newCondition.value = getDefaultNewCondition()
  draftParameter.value = defaultParam()
}

function deleteRule(conditionId: string) {
  store.deleteSemanticCondition(conditionId)
}
</script>
