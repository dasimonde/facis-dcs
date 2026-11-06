<script setup lang="ts">
import { computed } from 'vue'
import DiffPane from '@contract-workflow-engine/components/diff-view/DiffPane.vue'
import {
  buildWordDiffSegments,
  type TextDiffSegment,
  useContractBlockDiff,
} from '@contract-workflow-engine/composables/useContractBlockDiff'
import {
  type ContractPlainTextBlock,
  useContractPlainTextConverter,
} from '@contract-workflow-engine/composables/useContractPlainTextConverter'
import type { ContractData } from '@/models/contract/contract-data'
import type { ContractDetailsSnapshot } from '@contract-workflow-engine/models/contract-details-snapshot'

const props = withDefaults(
  defineProps<{
    leftContractData?: ContractData
    rightContractData?: ContractData
    leftDetails?: ContractDetailsSnapshot | null
    rightDetails?: ContractDetailsSnapshot | null
    showLineNumbers?: boolean
    highlightDiff?: boolean
    leftPaneTitle?: string
    rightPaneTitle?: string
    leftTestId?: string
    rightTestId?: string
  }>(),
  {
    showLineNumbers: true,
    highlightDiff: true,
    leftPaneTitle: '',
    rightPaneTitle: '',
    leftDetails: null,
    rightDetails: null,
    leftTestId: undefined,
    rightTestId: undefined,
  },
)

const { convertContractToPlainTextBlocks } = useContractPlainTextConverter()
const { buildContractBlockDiff } = useContractBlockDiff()

const hasLeftContractData = computed(() => !!props.leftContractData)

const leftBlocks = computed<ContractPlainTextBlock[]>(() => {
  if (!props.leftContractData) return []
  return convertContractToPlainTextBlocks(props.leftContractData)
})

const rightBlocks = computed<ContractPlainTextBlock[]>(() => {
  if (!props.rightContractData) return []
  return convertContractToPlainTextBlocks(props.rightContractData)
})

const contractDiffDocument = computed(() => buildContractBlockDiff(leftBlocks.value, rightBlocks.value))

const detailKeys: (keyof ContractDetailsSnapshot)[] = [
  'name',
  'contract_version',
  'description',
  'start_date',
  'exp_date',
  'exp_notice_period',
  'exp_policy',
  'signing_deadline',
  'signing_order',
]

function displayDetail(
  snapshot: ContractDetailsSnapshot | null | undefined,
  key: keyof ContractDetailsSnapshot,
): string {
  const value = snapshot?.[key]
  if (value == null || value === '') return ''
  if (key === 'start_date' || key === 'exp_date') {
    const date = new Date(String(value))
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
  }
  return String(value)
}

const changedDetailKeys = computed(() => {
  if (!props.highlightDiff) return [] as (keyof ContractDetailsSnapshot)[]
  return detailKeys.filter((key) => String(props.leftDetails?.[key] ?? '') !== String(props.rightDetails?.[key] ?? ''))
})

const leftDetailSegmentsByKey = computed(() => {
  const map: Partial<Record<keyof ContractDetailsSnapshot, TextDiffSegment[]>> = {}
  if (!props.highlightDiff) return map
  for (const key of changedDetailKeys.value) {
    const left = displayDetail(props.leftDetails, key)
    const right = displayDetail(props.rightDetails, key)
    const wordDiff = buildWordDiffSegments(left || '—', right || '—')
    map[key] = wordDiff.leftSegments?.length
      ? wordDiff.leftSegments
      : left
        ? [{ type: 'equal', text: left }]
        : [{ type: 'equal', text: '—' }]
  }
  return map
})

const rightDetailSegmentsByKey = computed(() => {
  const map: Partial<Record<keyof ContractDetailsSnapshot, TextDiffSegment[]>> = {}
  if (!props.highlightDiff) return map
  for (const key of changedDetailKeys.value) {
    const left = displayDetail(props.leftDetails, key)
    const right = displayDetail(props.rightDetails, key)
    const wordDiff = buildWordDiffSegments(left || '—', right || '—')
    map[key] = wordDiff.rightSegments?.length
      ? wordDiff.rightSegments
      : right
        ? [{ type: 'equal', text: right }]
        : [{ type: 'equal', text: '—' }]
  }
  return map
})
</script>

<template>
  <div class="grid min-h-128 grid-cols-1 gap-4 lg:grid-cols-2">
    <DiffPane
      :title="leftPaneTitle"
      :blocks="leftBlocks"
      :diff-rows="contractDiffDocument.leftRows"
      :highlight-diff="highlightDiff"
      :show-no-prior-version="!hasLeftContractData && !leftDetails"
      :show-line-numbers="showLineNumbers"
      :details="leftDetails"
      :detail-segments-by-key="leftDetailSegmentsByKey"
      :test-id="leftTestId"
    />
    <DiffPane
      :title="rightPaneTitle"
      :blocks="rightBlocks"
      :diff-rows="contractDiffDocument.rightRows"
      :highlight-diff="highlightDiff"
      :show-line-numbers="showLineNumbers"
      :details="rightDetails"
      :detail-segments-by-key="rightDetailSegmentsByKey"
      :test-id="rightTestId"
    />
  </div>
</template>
