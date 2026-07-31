<script setup lang="ts">
import { computed } from 'vue'
import {
  type ContractDiffRow,
  type DiffType,
  type TextDiffSegment,
} from '@contract-workflow-engine/composables/useContractBlockDiff'
import {
  type ContractPlainTextBlock,
  isSectionPlainTextBlock,
} from '@contract-workflow-engine/composables/useContractPlainTextConverter'
import DiffSectionBlock from './DiffSectionBlock.vue'
import DiffTextBlock from './DiffTextBlock.vue'

const props = withDefaults(
  defineProps<{
    title: string
    blocks: ContractPlainTextBlock[]
    diffRows?: ContractDiffRow[]
    highlightDiff?: boolean
    showNoPriorVersion?: boolean
    showLineNumbers?: boolean
  }>(),
  {
    diffRows: () => [],
    highlightDiff: false,
    showNoPriorVersion: false,
    showLineNumbers: true,
  },
)

const diffRowByLine = computed(() => {
  return new Map(props.diffRows.map((row) => [row.lineNumber, row]))
})

function getDiffTypeByLine(lineNumber: number): DiffType | null {
  if (!props.highlightDiff) return null
  return diffRowByLine.value.get(lineNumber)?.type ?? null
}

function getSegmentsByLine(lineNumber: number): TextDiffSegment[] | undefined {
  return diffRowByLine.value.get(lineNumber)?.segments
}

function shouldHighlightSegments(lineNumber: number): boolean {
  if (!props.highlightDiff) return false
  return getDiffTypeByLine(lineNumber) === 'modified'
}

function getRowBackgroundClass(lineNumber: number): string {
  const diffType = getDiffTypeByLine(lineNumber)
  if (diffType === 'added') return 'bg-green-100/70'
  if (diffType === 'removed') return 'bg-red-100/70'
  if (diffType === 'modified') return 'bg-amber-100/20'
  return ''
}
</script>

<template>
  <div class="card h-full min-h-0 border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body min-h-0 p-4">
      <div v-if="title.trim()" class="mb-2 text-sm font-semibold text-base-content/70">
        {{ title }}
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <div v-if="showNoPriorVersion" class="flex h-full items-center justify-center text-base-content/50">
          no prior version
        </div>
        <template v-else>
          <div
            v-for="(block, index) in blocks"
            :key="`${block.type}-${index}`"
            class="flex min-h-2 items-start"
            :class="getRowBackgroundClass(index + 1)"
          >
            <div
              v-if="showLineNumbers"
              class="relative mr-4 w-12 shrink-0 border-r border-base-300/60 pt-0 pr-2 text-right text-base leading-6 text-base-content/40 select-none"
            >
              <span class="block">{{ index + 1 }}</span>
            </div>
            <div class="min-w-0 flex-1">
              <DiffSectionBlock
                v-if="isSectionPlainTextBlock(block)"
                :block="block"
                :segments="getSegmentsByLine(index + 1)"
                :highlight-segments="shouldHighlightSegments(index + 1)"
              />
              <DiffTextBlock
                v-else
                :block="block"
                :segments="getSegmentsByLine(index + 1)"
                :highlight-segments="shouldHighlightSegments(index + 1)"
              />
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
