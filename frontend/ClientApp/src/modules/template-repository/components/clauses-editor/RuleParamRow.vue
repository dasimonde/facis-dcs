<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticConditionParameter } from '@/modules/template-repository/models/contract-template'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'

const props = defineProps<{
  param: SemanticConditionParameter
  isUsed?: boolean
  isRequiredAndUnused?: boolean
}>()

defineEmits<{
  mouseenter: []
  mouseleave: []
  click: []
}>()

const rowClass = computed(() => ({
  'text-primary font-medium': props.isUsed,
  'text-error cursor-pointer hover:bg-base-200': props.isRequiredAndUnused,
  'cursor-pointer hover:bg-base-200': !props.isRequiredAndUnused,
}))

const label = computed(() => semanticParameterLabel(props.param))

const constraintLabel = computed(() => {
  const constraint = props.param.valueConstraint
  if (!constraint) return ''
  if (constraint.allowedValuesRef) return constraint.allowedValuesRef
  if (constraint.format) return constraint.format
  if (constraint.allowedValues?.length) return constraint.allowedValues.join(', ')
  if (constraint.min !== undefined || constraint.max !== undefined)
    return `${constraint.min ?? '-'}-${constraint.max ?? '-'}`
  return ''
})
</script>

<template>
  <li class="-mx-1 flex items-center gap-1.5 rounded px-1 py-0.5" :class="rowClass" @click="$emit('click')">
    <span
      class="rounded border border-base-300 px-1"
      @mouseenter="$emit('mouseenter')"
      @mouseleave="$emit('mouseleave')"
    >
      {{ label }}
    </span>
    <span class="text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
    <span v-if="constraintLabel" class="text-base-content/40">{{ constraintLabel }}</span>
  </li>
</template>
