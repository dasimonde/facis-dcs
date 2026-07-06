<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { VerificationResult } from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticCondition } from '@template-repository/models/contract-template'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { DcsBlock, DcsLayoutNode, DcsContentSegment } from '@/models/dcs-jsonld'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'
import { isDcsMergedApprovedTemplate } from '@template-repository/store/dcsDraftStore'
import {
  getBlocksFromTemplateData,
  getLayoutFromTemplateData,
  getSemanticConditionsFromTemplateData,
} from '@template-repository/store/dcsDraftStore'
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
    /** If blockId is provided, the preview will render only that block and its children. */
    blockId?: string
    /** Section nesting level for headings (1 = top-level) */
    sectionLevel?: number
    layout: DcsLayoutNode[]
    blocks: (DcsBlock | MergedApprovedTemplateBlock)[]
    semanticConditions: SemanticCondition[]
    subTemplateSnapshots?: SubTemplateSnapshot[]
    semanticConditionValues?: SemanticConditionValue[]
    verificationResult?: VerificationResult | null
    setSemanticConditionValue?: SemanticConditionValueSetter
  }>(),
  { sectionLevel: 1, semanticConditionValues: () => [], verificationResult: null, setSemanticConditionValue: null },
)

const hasBlockId = computed(() => props.blockId != null)

const rootChildren = computed(() => {
  const root = props.layout.find((n) => n['dcs:isRoot'])
  return root ? root['dcs:children']['@list'].map((r) => r['@id']) : []
})

const block = computed<DcsBlock | MergedApprovedTemplateBlock | undefined>(() => {
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

const clauseSemanticConditions = computed(() => {
  if (!isMergedBlockId(props.blockId ?? '')) return props.semanticConditions
  return subTemplateSemanticConditions.value
})

const subTemplate = computed((): SubTemplateSnapshot | undefined => {
  const b = block.value
  if (!b) return
  if (!props.subTemplateSnapshots?.length) return undefined
  if (isDcsMergedApprovedTemplate(b)) {
    return props.subTemplateSnapshots.find((snapshot) =>
      isSameTemplateDataRef(
        { templateId: snapshot.did, version: snapshot.version, document_number: snapshot.document_number },
        { templateId: b['dcs:templateDid'], version: b['dcs:version'], document_number: b['dcs:documentNumber'] },
      ),
    )
  }
  if (isMergedBlockId(b['@id'])) {
    const mergedOwnerBlockId = getOwnerBlockIdFromMergedBlockId(b['@id'])
    const mergedBlock = mergedOwnerBlockId ? props.blocks.find((c) => c['@id'] === mergedOwnerBlockId) : undefined
    if (mergedBlock && isDcsMergedApprovedTemplate(mergedBlock)) {
      return props.subTemplateSnapshots.find((snapshot) =>
        isSameTemplateDataRef(
          { templateId: snapshot.did, version: snapshot.version, document_number: snapshot.document_number },
          {
            templateId: mergedBlock['dcs:templateDid'],
            version: mergedBlock['dcs:version'],
            document_number: mergedBlock['dcs:documentNumber'],
          },
        ),
      )
    }
  }
  if (b['@type'] !== 'dcs:ApprovedTemplate') return undefined
  return props.subTemplateSnapshots.find((snapshot) =>
    isSameTemplateDataRef(
      { templateId: snapshot.did, version: snapshot.version, document_number: snapshot.document_number },
      { templateId: b['dcs:templateDid'], version: b['dcs:version'], document_number: b['dcs:documentNumber'] },
    ),
  )
})

const subTemplateBlocks = computed(() => getBlocksFromTemplateData(subTemplate.value?.template_data))
const subTemplateLayout = computed(() => getLayoutFromTemplateData(subTemplate.value?.template_data))
const subTemplateSemanticConditions = computed(() =>
  getSemanticConditionsFromTemplateData(subTemplate.value?.template_data),
)
const hasApprovedTemplateChildren = computed(
  () => block.value?.['@type'] === 'dcs:ApprovedTemplate' && childrenIds.value.length > 0,
)
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
            :sub-template-snapshots="subTemplateSnapshots"
            :semantic-condition-values="semanticConditionValues"
            :verification-result="verificationResult"
            :set-semantic-condition-value="setSemanticConditionValue"
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
    />
    <!-- Approved template block -->
    <ConditionalWrapper
      v-else-if="block && block['@type'] === 'dcs:ApprovedTemplate'"
      :enabled="hasApprovedTemplateChildren"
    >
      <TemplatePreview
        v-if="subTemplate?.template_data"
        :layout="subTemplateLayout"
        :blocks="subTemplateBlocks"
        :semantic-conditions="subTemplateSemanticConditions"
        :sub-template-snapshots="subTemplateSnapshots"
        :sub-block-id="block['@id']"
        :section-level="sectionLevel"
        :semantic-condition-values="semanticConditionValues"
        :verification-result="verificationResult"
        :set-semantic-condition-value="setSemanticConditionValue"
      />
      <template v-for="childId in childrenIds" :key="childId">
        <TemplatePreview
          :block-id="childId"
          :section-level="sectionLevel + 1"
          :layout="layout"
          :blocks="blocks"
          :semantic-conditions="semanticConditions"
          :sub-template-snapshots="subTemplateSnapshots"
          :semantic-condition-values="semanticConditionValues"
          :verification-result="verificationResult"
          :set-semantic-condition-value="setSemanticConditionValue"
        />
      </template>
    </ConditionalWrapper>
    <!-- Merged approved template block (preprocessed contract view) — content already merged into main layout/blocks -->
    <template v-else-if="block && isDcsMergedApprovedTemplate(block)">
      <template v-for="childId in childrenIds" :key="childId">
        <TemplatePreview
          :block-id="childId"
          :section-level="sectionLevel"
          :layout="layout"
          :blocks="blocks"
          :semantic-conditions="semanticConditions"
          :sub-template-snapshots="subTemplateSnapshots"
          :semantic-condition-values="semanticConditionValues"
          :verification-result="verificationResult"
          :set-semantic-condition-value="setSemanticConditionValue"
        />
      </template>
    </template>
  </template>
</template>
