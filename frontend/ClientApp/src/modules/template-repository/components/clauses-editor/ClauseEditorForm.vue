<template>
  <!-- <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm"> -->
  <h3 class="mb-4 text-sm font-semibold text-base-content/80">
    {{ mode === 'edit' ? 'Edit clause' : 'New clause' }}
  </h3>
  <div class="space-y-4">
    <div>
      <label class="label-text mb-1 block text-xs text-base-content/60">
        Clause title
        <RequiredIndicator />
      </label>
      <input v-model="localTitle" type="text" class="input-bordered input input-sm w-full" placeholder="" required />
    </div>
    <div>
      <label class="label-text mb-1 block text-xs text-base-content/60">
        Clause text
        <RequiredIndicator />
      </label>
      <ClauseTextEditor
        :model-value="localText"
        :semantic-conditions="semanticConditions"
        @update:model-value="localText = $event"
      />
    </div>
    <div class="flex items-center justify-between">
      <button v-if="mode === 'edit'" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">
        Cancel
      </button>
      <span v-else />
      <button type="button" class="btn btn-sm btn-secondary" :disabled="!canSubmit" @click="handleSubmit">
        {{ mode === 'edit' ? 'Save changes' : 'Add clause' }}
      </button>
    </div>
  </div>
  <!-- </section> -->
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SemanticCondition } from '@template-repository/models/contract-templace'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import ClauseTextEditor from '@template-repository/components/clauses-editor/ClauseTextEditor.vue'
import { conditionIdsInText, isPlaceholder, parseSegments } from '@template-repository/composables/useClauseTextChips'

const props = defineProps<{
  mode: 'create' | 'edit'
  initialTitle: string
  initialText: string
  semanticConditions: SemanticCondition[]
}>()

const emit = defineEmits<{
  submit: [payload: { title: string; text: string }]
  cancel: []
}>()

const localTitle = ref(props.initialTitle)
const localText = ref(props.initialText)

watch(
  () => [props.initialTitle, props.initialText] as const,
  ([title, text]) => {
    localTitle.value = title
    localText.value = text
  },
)

// Check if there is any required parameter in used rule panel that is not used in the text
const hasRequiredUnusedParamInUsedRules = computed(() => {
  const usedConditionIds = conditionIdsInText(localText.value)
  if (!usedConditionIds.size) return false

  const usedParams = new Set<string>()
  parseSegments(localText.value, props.semanticConditions)
    .filter((segment) => isPlaceholder(segment))
    .forEach((segment) => {
      usedParams.add(`${segment.conditionId}.${segment.parameterName}`)
    })

  return props.semanticConditions
    .filter((c) => usedConditionIds.has(c.conditionId))
    .some((c) => c.parameters.some((p) => p.isRequired && !usedParams.has(`${c.conditionId}.${p.parameterName}`)))
})

const canSubmit = computed(
  () => !!localTitle.value.trim() && !!localText.value.trim() && !hasRequiredUnusedParamInUsedRules.value,
)

function handleSubmit() {
  if (!canSubmit.value) return
  emit('submit', {
    title: localTitle.value.trim(),
    text: localText.value,
  })
  if (props.mode === 'create') {
    localTitle.value = ''
    localText.value = ''
  }
}
</script>
