<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import ClauseTextEditor from '@template-repository/components/clauses-editor/ClauseTextEditor.vue'
import {
  conditionIdsInContent,
  usedPlaceholderKeysInContent,
} from '@template-repository/composables/useClauseTextChips'
import type { DcsContentSegment } from '@/models/dcs-jsonld'
import type { SemanticCondition } from '@template-repository/models/contract-template'

const props = defineProps<{
  mode: 'create' | 'edit'
  initialTitle: string
  initialContent?: DcsContentSegment[]
  semanticConditions: SemanticCondition[]
  sourceRequirementName?: string
  showCancel?: boolean
}>()

const emit = defineEmits<{
  submit: [payload: { title: string; content: DcsContentSegment[] }]
  cancel: []
}>()

const localTitle = ref(props.initialTitle)
const localContent = ref<DcsContentSegment[]>(props.initialContent ?? [])

const localTitleId = useId()
const localContentId = useId()

const heading = computed(() => {
  if (props.mode === 'edit') return 'Edit clause'
  return props.sourceRequirementName ? `Create clause from ${props.sourceRequirementName}` : 'New clause'
})

watch(
  () => [props.initialTitle, props.initialContent] as const,
  ([title, content]) => {
    localTitle.value = title
    localContent.value = content ?? []
  },
)

const hasRequiredUnusedParamInUsedRules = computed(() => {
  const usedConditionIds = conditionIdsInContent(localContent.value, props.semanticConditions)
  if (!usedConditionIds.size) return false
  const usedKeys = usedPlaceholderKeysInContent(localContent.value, props.semanticConditions)
  return props.semanticConditions
    .filter((c) => usedConditionIds.has(c.conditionId))
    .some((c) => c.parameters.some((p) => p.isRequired && !usedKeys.has(`${c.conditionId}.${p.parameterName}`)))
})

const canSubmit = computed(
  () =>
    !!localTitle.value.trim() &&
    localContent.value.some((seg) => (typeof seg === 'string' ? seg.trim() : true)) &&
    !hasRequiredUnusedParamInUsedRules.value,
)

function handleSubmit() {
  if (!canSubmit.value) return
  emit('submit', {
    title: localTitle.value.trim(),
    content: localContent.value,
  })
  if (props.mode === 'create') {
    localTitle.value = ''
    localContent.value = []
  }
}
</script>

<template>
  <h3 class="mb-4 text-sm font-semibold text-base-content/80">
    {{ heading }}
  </h3>
  <div class="space-y-4">
    <div>
      <label :for="localTitleId" class="label-text mb-1 block text-xs text-base-content/70">
        Clause title
        <RequiredIndicator />
      </label>
      <input
        :id="localTitleId"
        v-model="localTitle"
        type="text"
        class="input-bordered input input-sm w-full"
        placeholder=""
        required
      />
    </div>
    <div>
      <span :id="localContentId" class="label-text mb-1 block text-xs text-base-content/70">
        Clause text
        <RequiredIndicator />
      </span>
      <ClauseTextEditor
        :text-id="localContentId"
        :model-value="localContent"
        :semantic-conditions="semanticConditions"
        @update:model-value="localContent = $event"
      />
    </div>
    <div class="flex items-center justify-between">
      <button v-if="mode === 'edit'" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">
        Cancel
      </button>
      <button v-else-if="showCancel" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">
        Cancel
      </button>
      <span v-else />
      <button type="button" class="btn btn-sm btn-secondary" :disabled="!canSubmit" @click="handleSubmit">
        {{ mode === 'edit' ? 'Save changes' : 'Add clause' }}
      </button>
    </div>
  </div>
</template>
