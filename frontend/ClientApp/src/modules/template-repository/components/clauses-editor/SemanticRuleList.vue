<script setup lang="ts">
import type { SemanticCondition } from '@/modules/template-repository/models/contract-template'
import RuleParamRow from './RuleParamRow.vue'

withDefaults(
  defineProps<{
    title: string
    emptyMessage: string
    conditions: SemanticCondition[]
    isParamUsedInText?: (conditionId: string, parameterName: string) => boolean
    isParamRequiredAndUnused?: (conditionId: string, parameterName: string) => boolean
    highlightRuleTitle?: boolean
  }>(),
  {
    isParamUsedInText: () => false,
    isParamRequiredAndUnused: () => false,
    highlightRuleTitle: false,
  },
)

const emit = defineEmits<{
  highlightRule: [conditionId: string]
  highlightParam: [conditionId: string, parameterName: string]
  clearHighlight: []
  insertPlaceholder: [conditionId: string, parameterName: string]
}>()

function onRuleEnter(conditionId: string) {
  emit('highlightRule', conditionId)
}

function onRuleLeave() {
  emit('clearHighlight')
}

function onParamLeave() {
  emit('clearHighlight')
}

function onParamEnter(conditionId: string, parameterName: string) {
  emit('highlightParam', conditionId, parameterName)
}

function onParamClick(conditionId: string, parameterName: string) {
  emit('insertPlaceholder', conditionId, parameterName)
}
</script>

<template>
  <section class="rounded-lg border border-base-300 bg-base-100 p-3">
    <h4 class="mb-2 text-xs font-semibold text-base-content/70">{{ title }}</h4>
    <p v-if="!conditions.length" class="text-xs text-base-content/50 italic">{{ emptyMessage }}</p>
    <ul v-else class="space-y-2">
      <li v-for="c in conditions" :key="c.conditionId" class="text-xs">
        <span
          class="font-medium"
          :class="{ 'text-primary': highlightRuleTitle }"
          @mouseenter="onRuleEnter(c.conditionId)"
          @mouseleave="onRuleLeave"
        >
          {{ c.conditionName }}
        </span>
        <ul class="mt-1 ml-3 space-y-0.5">
          <RuleParamRow
            v-for="p in c.parameters"
            :key="p.parameterName"
            :param="p"
            :is-used="isParamUsedInText(c.conditionId, p.parameterName)"
            :is-required-and-unused="isParamRequiredAndUnused(c.conditionId, p.parameterName)"
            @mouseenter="onParamEnter(c.conditionId, p.parameterName)"
            @mouseleave="onParamLeave"
            @click="onParamClick(c.conditionId, p.parameterName)"
          />
        </ul>
      </li>
    </ul>
  </section>
</template>
