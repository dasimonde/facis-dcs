<script setup lang="ts">
import { computed } from 'vue'
import type { TextDiffSegmentType } from '@contract-workflow-engine/composables/useContractBlockDiff'
import type { TextDiffSegment } from '@contract-workflow-engine/composables/useContractBlockDiff'
import type { ContractPlainTextLine } from '@contract-workflow-engine/composables/useContractPlainTextConverter'

const props = withDefaults(
  defineProps<{
    block: ContractPlainTextLine
    segments?: TextDiffSegment[]
    highlightSegments?: boolean
  }>(),
  {
    segments: () => [],
    highlightSegments: false,
  },
)

const hasSegments = computed(() => props.segments.length > 0)

function getSegmentClass(type: TextDiffSegmentType): string {
  if (type === 'added') return 'bg-green-200/80 text-green-900 rounded-sm'
  if (type === 'removed') return 'bg-red-200/80 text-red-900 rounded-sm'
  return ''
}
</script>

<template>
  <p class="m-0 leading-6 whitespace-pre-wrap text-base-content/90">
    <template v-if="highlightSegments && hasSegments">
      <span v-for="(segment, index) in segments" :key="`text-segment-${index}`" :class="getSegmentClass(segment.type)">
        {{ segment.text }}
      </span>
    </template>
    <template v-else>
      {{ block.text }}
    </template>
  </p>
</template>
