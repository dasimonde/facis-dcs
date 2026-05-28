<template>
  <!-- Root-level blocks -->
  <template v-if="!hasBlockId">
    <template v-for="id in rootChildren" :key="id">
      <TemplatePreview
        :block-id="id"
        :section-level="sectionLevel"
        :document-outline="documentOutline"
        :document-blocks="documentBlocks"
        :semantic-conditions="semanticConditions"
        :sub-template-snapshots="subTemplateSnapshots"
        :semantic-condition-values="semanticConditionValues"
        :verification-result="verificationResult"
        :set-semantic-condition-value="setSemanticConditionValue"
      />
    </template>
  </template>
  <!-- Nested blocks -->
  <template v-else>
    <!-- Section block -->
    <ConditionalWrapper v-if="block && isSection" :enabled="true" tag="section" wrapper-class="w-full mb-4">
      <PreviewSectionBlock :title="sectionTitle" :has-children="childrenIds.length > 0" :level="sectionLevel">
        <template v-for="childId in childrenIds" :key="childId">
          <TemplatePreview
            :block-id="childId"
            :section-level="sectionLevel + 1"
            :document-outline="documentOutline"
            :document-blocks="documentBlocks"
            :semantic-conditions="semanticConditions"
            :sub-template-snapshots="subTemplateSnapshots"
            :semantic-condition-values="semanticConditionValues"
            :verification-result="verificationResult"
            :set-semantic-condition-value="setSemanticConditionValue"
          />
        </template>
      </PreviewSectionBlock>
    </ConditionalWrapper>
    <!-- Text block -->
    <PreviewTextBlock v-else-if="block && isText" :text="block.text ?? ''" />
    <!-- Clause block -->
    <PreviewClauseBlock
      v-else-if="block && isClause"
      :block-id="block.blockId"
      :text="block.text ?? ''"
      :semantic-conditions="clauseSemanticConditions"
      :semantic-condition-values="semanticConditionValues"
      :verification-result="verificationResult"
      :set-semantic-condition-value="setSemanticConditionValue"
    />
    <!-- Approved template block -->
    <ConditionalWrapper v-else-if="block && isApprovedTemplate" :enabled="hasApprovedTemplateChildren">
      <TemplatePreview
        v-if="subTemplate?.template_data"
        :document-outline="subTemplate.template_data.documentOutline"
        :document-blocks="subTemplate.template_data.documentBlocks"
        :semantic-conditions="subTemplate.template_data.semanticConditions"
        :sub-template-snapshots="subTemplateSnapshots"
        :sub-block-id="block.blockId"
        :section-level="sectionLevel"
        :semantic-condition-values="semanticConditionValues"
        :verification-result="verificationResult"
        :set-semantic-condition-value="setSemanticConditionValue"
      />
      <template v-for="childId in childrenIds" :key="childId">
        <TemplatePreview
          :block-id="childId"
          :section-level="sectionLevel + 1"
          :document-outline="documentOutline"
          :document-blocks="documentBlocks"
          :semantic-conditions="semanticConditions"
          :sub-template-snapshots="subTemplateSnapshots"
          :semantic-condition-values="semanticConditionValues"
          :verification-result="verificationResult"
          :set-semantic-condition-value="setSemanticConditionValue"
        />
      </template>
    </ConditionalWrapper>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { VerificationResult } from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type {
  DocumentBlock,
  DocumentOutline,
  SectionBlock,
  SemanticCondition,
} from '@template-repository/models/contract-templace'
import {
  isSectionBlock,
  isTextBlock,
  isClauseBlock,
  isApprovedTemplateBlock,
  isMergedApprovedTemplateBlock,
} from '@template-repository/models/contract-templace'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import ConditionalWrapper from '@/core/components/ConditionalWrapper.vue'
import PreviewSectionBlock from './PreviewSectionBlock.vue'
import PreviewTextBlock from './PreviewTextBlock.vue'
import PreviewClauseBlock from './PreviewClauseBlock.vue'
import {
  getOwnerBlockIdFromMergedBlockId,
  isMergedBlockId,
  isSameTemplateDataRef,
} from '@template-repository/utils/template-data-ref'

const props = withDefaults(
  defineProps<{
    /** If blockId is provided, the preview will render only that block and its children.
     *  If not provided, it will render all root-level blocks.
     */
    blockId?: string
    /** Section nesting level for headings (1 = top-level) */
    sectionLevel?: number
    documentOutline: DocumentOutline
    documentBlocks: DocumentBlock[]
    semanticConditions: SemanticCondition[]
    subTemplateSnapshots?: SubTemplateSnapshot[]
    semanticConditionValues?: SemanticConditionValue[]
    verificationResult?: VerificationResult | null
    setSemanticConditionValue?: SemanticConditionValueSetter
  }>(),
  { sectionLevel: 1, semanticConditionValues: () => [], verificationResult: null, setSemanticConditionValue: null },
)
const hasBlockId = computed(() => props.blockId != null)

const documentOutline = computed(() => props.documentOutline)
const documentBlocks = computed(() => props.documentBlocks)
const semanticConditions = computed(() => props.semanticConditions)
const clauseSemanticConditions = computed(() => {
  if (!isMergedBlockId(props.blockId ?? '')) return props.semanticConditions
  return subTemplate.value?.template_data?.semanticConditions ?? []
})
const semanticConditionValues = computed(() => props.semanticConditionValues)
const verificationResult = computed(() => props.verificationResult)
const setSemanticConditionValue = computed(() => props.setSemanticConditionValue)

const rootChildren = computed(() => {
  const root = documentOutline.value.find((b) => b.isRoot)
  return root?.children ?? []
})

const block = computed<DocumentBlock | undefined>(() => {
  if (!props.blockId) return undefined
  return documentBlocks.value.find((b) => b.blockId === props.blockId)
})

const outlineNode = computed(() => {
  if (!props.blockId) return undefined
  return documentOutline.value.find((o) => o.blockId === props.blockId)
})

const childrenIds = computed(() => outlineNode.value?.children ?? [])

const isSection = computed(() => !!block.value && isSectionBlock(block.value))
const isText = computed(() => !!block.value && isTextBlock(block.value))
const isClause = computed(() => !!block.value && isClauseBlock(block.value))
const isApprovedTemplate = computed(() => !!block.value && isApprovedTemplateBlock(block.value))

const sectionTitle = computed(() => {
  const b = block.value as SectionBlock | undefined
  return b?.title ?? b?.text ?? ''
})

const sectionLevel = computed(() => props.sectionLevel ?? 1)
const subTemplate = computed((): SubTemplateSnapshot | undefined => {
  const b = block.value
  if (!b) return
  if (!props.subTemplateSnapshots?.length) return undefined
  if (isMergedApprovedTemplateBlock(b)) {
    return props.subTemplateSnapshots.find((snapshot) =>
      isSameTemplateDataRef(
        {
          templateId: snapshot.did,
          version: snapshot.version,
          document_number: snapshot.document_number,
        },
        {
          templateId: b.templateId,
          version: b.version,
          document_number: b.document_number,
        },
      ),
    )
  }
  if (isMergedBlockId(b.blockId)) {
    const mergedOwnerBlockId = getOwnerBlockIdFromMergedBlockId(b.blockId)
    const mergedBlock = props.documentBlocks.find((candidate) => candidate.blockId === mergedOwnerBlockId)
    if (mergedBlock && isMergedApprovedTemplateBlock(mergedBlock)) {
      return props.subTemplateSnapshots.find((snapshot) =>
        isSameTemplateDataRef(
          {
            templateId: snapshot.did,
            version: snapshot.version,
            document_number: snapshot.document_number,
          },
          {
            templateId: mergedBlock.templateId,
            version: mergedBlock.version,
            document_number: mergedBlock.document_number,
          },
        ),
      )
    }
  }
  if (!isApprovedTemplateBlock(b)) return undefined
  return props.subTemplateSnapshots.find((snapshot) =>
    isSameTemplateDataRef(
      {
        templateId: snapshot.did,
        version: snapshot.version,
        document_number: snapshot.document_number,
      },
      {
        templateId: b.templateId,
        version: b.version,
        document_number: b.document_number,
      },
    ),
  )
})
const hasApprovedTemplateChildren = computed(() => isApprovedTemplate.value && childrenIds.value.length > 0)
</script>
