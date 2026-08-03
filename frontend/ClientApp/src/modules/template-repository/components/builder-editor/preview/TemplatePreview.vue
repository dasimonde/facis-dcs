<script setup lang="ts">
import { computed } from 'vue'
import ConditionalWrapper from '@core/components/ConditionalWrapper.vue'
import PreviewClauseBlock from './PreviewClauseBlock.vue'
import PreviewSectionBlock from './PreviewSectionBlock.vue'
import PreviewTextBlock from './PreviewTextBlock.vue'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsBlock, DcsContentSegment, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { VerificationResult } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'
import type { SemanticCondition } from '@template-repository/models/contract-template'

const props = withDefaults(
  defineProps<{
    /** If blockId is provided, the preview will render only that block and its children. */
    blockId?: string
    /** Section nesting level for headings (1 = top-level) */
    sectionLevel?: number
    layout: DcsLayoutNode[]
    blocks: DcsBlock[]
    semanticConditions: SemanticCondition[]
    semanticConditionValues?: SemanticConditionValue[]
    verificationResult?: VerificationResult | null
    setSemanticConditionValue?: SemanticConditionValueSetter
    disabled?: boolean
  }>(),
  {
    sectionLevel: 1,
    semanticConditionValues: () => [],
    verificationResult: null,
    setSemanticConditionValue: null,
    disabled: false,
  },
)

const hasBlockId = computed(() => props.blockId != null)

const rootChildren = computed(() => {
  const root = props.layout.find((n) => n['dcs:isRoot'])
  return root ? root['dcs:children']['@list'].map((r) => r['@id']) : []
})

const block = computed<DcsBlock | undefined>(() => {
  if (!props.blockId) return undefined
  return props.blocks.find((b) => b['@id'] === props.blockId)
})

const outlineNode = computed(() => {
  if (!props.blockId) return undefined
  return props.layout.find((n) => n['@id'] === props.blockId)
})

const childrenIds = computed(() => outlineNode.value?.['dcs:children']['@list'].map((r) => r['@id']) ?? [])

const sectionLevel = computed(() => props.sectionLevel ?? 1)

const sectionTitle = computed(() => {
  const b = block.value
  if (b?.['@type'] !== 'dcs:Section') return ''
  return b['dcs:title'] ?? ''
})

const textBlockText = computed(() => {
  const b = block.value
  if (b?.['@type'] !== 'dcs:TextBlock') return ''
  return b['dcs:text'] ?? ''
})

const clauseContent = computed((): DcsContentSegment[] => {
  const b = block.value
  if (b?.['@type'] !== 'dcs:Clause') return []
  const content = b['dcs:content']
  if (typeof content === 'string') return []
  return content['@list']
})

// Every ContractField is a self-contained top-level declaration wired by
// @id, so a clause always resolves against the top-level conditions.
const clauseSemanticConditions = computed(() => props.semanticConditions)
</script>

<template>
  <!-- Root-level blocks -->
  <template v-if="!hasBlockId">
    <template v-for="id in rootChildren" :key="id">
      <TemplatePreview
        :block-id="id"
        :section-level="sectionLevel"
        :layout="layout"
        :blocks="blocks"
        :semantic-conditions="semanticConditions"
        :semantic-condition-values="semanticConditionValues"
        :verification-result="verificationResult"
        :set-semantic-condition-value="setSemanticConditionValue"
        :disabled="disabled"
      />
    </template>
  </template>
  <!-- Nested blocks -->
  <template v-else>
    <!-- Section block -->
    <ConditionalWrapper
      v-if="block && block['@type'] === 'dcs:Section'"
      :enabled="true"
      tag="section"
      wrapper-class="w-full mb-4"
    >
      <PreviewSectionBlock :title="sectionTitle" :has-children="childrenIds.length > 0" :level="sectionLevel">
        <template v-for="childId in childrenIds" :key="childId">
          <TemplatePreview
            :block-id="childId"
            :section-level="sectionLevel + 1"
            :layout="layout"
            :blocks="blocks"
            :semantic-conditions="semanticConditions"
            :semantic-condition-values="semanticConditionValues"
            :verification-result="verificationResult"
            :set-semantic-condition-value="setSemanticConditionValue"
            :disabled="disabled"
          />
        </template>
      </PreviewSectionBlock>
    </ConditionalWrapper>
    <!-- Text block -->
    <PreviewTextBlock v-else-if="block && block['@type'] === 'dcs:TextBlock'" :text="textBlockText" />
    <!-- Clause block -->
    <PreviewClauseBlock
      v-else-if="block && block['@type'] === 'dcs:Clause'"
      :block-id="block['@id']"
      :content="clauseContent"
      :semantic-conditions="clauseSemanticConditions"
      :semantic-condition-values="semanticConditionValues"
      :verification-result="verificationResult"
      :set-semantic-condition-value="setSemanticConditionValue"
      :disabled="disabled"
    />
  </template>
</template>
