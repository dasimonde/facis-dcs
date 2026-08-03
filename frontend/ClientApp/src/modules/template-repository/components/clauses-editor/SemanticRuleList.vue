<script setup lang="ts">
import { computed } from 'vue'
import RuleParamRow from './RuleParamRow.vue'
import type { SemanticCondition } from '@template-repository/models/contract-template'

const props = withDefaults(
  defineProps<{
    title: string
    emptyMessage: string
    conditions: SemanticCondition[]
    isParamUsedInText?: (conditionId: string, parameterName: string) => boolean
    isParamRequiredAndUnused?: (conditionId: string, parameterName: string) => boolean
    highlightRuleTitle?: boolean
    isConditionUsedInOtherClauses?: (conditionId: string) => boolean
  }>(),
  {
    isParamUsedInText: () => false,
    isParamRequiredAndUnused: () => false,
    highlightRuleTitle: false,
    isConditionUsedInOtherClauses: () => false,
  },
)

const visibleConditions = computed(() =>
  props.conditions.filter((condition) => !props.isConditionUsedInOtherClauses?.(condition.conditionId)),
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
    <p v-if="!visibleConditions.length" class="text-xs text-base-content/70 italic">{{ emptyMessage }}</p>
    <ul v-else class="space-y-2">
      <li v-for="c in visibleConditions" :key="c.conditionId" class="text-xs">
        <span
          class="font-medium"
          :class="{ 'text-primary': highlightRuleTitle }"
          @mouseenter="onRuleEnter(c.conditionId)"
          @mouseleave="onRuleLeave"
        >
          <span
            v-if="c.accentColor"
            class="mr-1 inline-block h-2 w-2 rounded-full align-baseline"
            :style="{ backgroundColor: c.accentColor }"
          />
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
