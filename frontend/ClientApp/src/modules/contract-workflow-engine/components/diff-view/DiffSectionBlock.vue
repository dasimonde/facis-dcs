<template>
  <p
    class="m-0 leading-6 font-semibold wrap-break-word whitespace-pre-wrap text-base-content"
    :class="sectionTextClass"
  >
    <template v-if="highlightSegments && hasSegments">
      <span
        v-for="(segment, index) in segments"
        :key="`section-segment-${index}`"
        :class="getSegmentClass(segment.type)"
      >
        {{ segment.text }}
      </span>
    </template>
    <template v-else>
      {{ block.text }}
    </template>
  </p>
</template>

<script setup lang="ts">
import type { TextDiffSegmentType } from '@/modules/contract-workflow-engine/composables/useContractBlockDiff'
import type { TextDiffSegment } from '@/modules/contract-workflow-engine/composables/useContractBlockDiff'
import type { ContractPlainTextSection } from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    block: ContractPlainTextSection
    segments?: TextDiffSegment[]
    highlightSegments?: boolean
  }>(),
  {
    segments: () => [],
    highlightSegments: false,
  },
)

const sectionTextClass = computed(() => {
  if (props.block.level <= 1) return 'text-lg'
  if (props.block.level === 2) return 'text-[17px]'
  return 'text-base'
})

const hasSegments = computed(() => props.segments.length > 0)

function getSegmentClass(type: TextDiffSegmentType): string {
  if (type === 'added') return 'bg-green-200/80 text-green-900 rounded-sm'
  if (type === 'removed') return 'bg-red-200/80 text-red-900 rounded-sm'
  return ''
}
</script>
