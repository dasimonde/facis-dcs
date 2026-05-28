<template>
  <div class="grid min-h-128 grid-cols-1 gap-4 lg:grid-cols-2">
    <DiffPane
      :title="leftPaneTitle"
      :blocks="leftBlocks"
      :diff-rows="contractDiffDocument.leftRows"
      :highlight-diff="highlightDiff"
      :show-no-prior-version="!hasLeftContractData"
      :show-line-numbers="showLineNumbers"
    />
    <DiffPane
      :title="rightPaneTitle"
      :blocks="rightBlocks"
      :diff-rows="contractDiffDocument.rightRows"
      :highlight-diff="highlightDiff"
      :show-line-numbers="showLineNumbers"
    />
  </div>
</template>

<script setup lang="ts">
import type { ContractData } from '@/models/contract-data'
import {
  type ContractPlainTextBlock,
  useContractPlainTextConverter,
} from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import { useContractBlockDiff } from '@/modules/contract-workflow-engine/composables/useContractBlockDiff'
import DiffPane from '@/modules/contract-workflow-engine/components/diff-view/DiffPane.vue'
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    leftContractData?: ContractData
    rightContractData?: ContractData
    showLineNumbers?: boolean
    highlightDiff?: boolean
    leftPaneTitle?: string
    rightPaneTitle?: string
  }>(),
  {
    showLineNumbers: true,
    highlightDiff: true,
    leftPaneTitle: '',
    rightPaneTitle: '',
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
</script>
