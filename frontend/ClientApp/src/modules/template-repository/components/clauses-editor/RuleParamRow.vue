<template>
  <li class="-mx-1 flex items-center gap-1.5 rounded px-1 py-0.5" :class="rowClass" @click="$emit('click')">
    <span
      class="rounded border border-base-300 px-1 font-mono"
      @mouseenter="$emit('mouseenter')"
      @mouseleave="$emit('mouseleave')"
    >
      {{ param.parameterName }}
    </span>
    <span class="text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
    <span class="text-base-content/40">({{ param.type }})</span>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticConditionParameter } from '@template-repository/models/contract-templace'

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
}))
</script>
