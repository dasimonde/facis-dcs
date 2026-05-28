<template>
  <Teleport to="body">
    <div
      v-if="addBlockModalContext !== null"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-block-title"
      @click.self="handleCancel"
    >
      <div
        class="mx-4 flex max-h-[85vh] w-full max-w-2xl flex-col gap-4 overflow-y-auto rounded-2xl bg-base-100 p-6 shadow-xl"
        @click.stop
      >
        <h2 id="add-block-title" class="text-lg font-bold">Add block</h2>
        <template v-if="!isContractWorkflow && isFrameContract">
          <ApprovedSubTemplatePicker
            :templates="subTemplateSnapshots"
            :reference-count-by-did="referenceCountByDid"
            @select="handleAddApprovedTemplate"
          />
        </template>
        <template v-else>
          <div>
            <p class="mb-2 text-sm text-base-content/70">Common:</p>
            <div class="flex flex-col gap-2">
              <BlockPaletteItem
                v-for="item in paletteBlockTypes"
                :key="item.blockType"
                :label="item.label"
                @select="handleAddBlock(item.blockType)"
              />
            </div>
          </div>

          <div v-if="unusedClauses.length" class="border-t border-base-300 pt-4">
            <p class="mb-2 text-sm text-base-content/70">Unused Clauses:</p>
            <div class="flex max-h-64 flex-col gap-2 overflow-y-auto">
              <button
                v-for="clause in unusedClauses"
                :key="clause.blockId"
                type="button"
                class="flex min-h-11 cursor-pointer flex-col justify-center rounded-lg border border-base-300 bg-base-100 px-3 py-2 text-left transition-colors select-none hover:bg-base-200"
                @click="handleAddClause(clause.blockId)"
              >
                <span class="text-sm font-medium text-base-content">{{ clause.title || 'Untitled clause' }}</span>
                <p class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-base-content/70">
                  <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
                </p>
              </button>
            </div>
          </div>
        </template>

        <div class="flex justify-end pt-2">
          <button type="button" class="btn btn-outline btn-sm" @click="handleCancel">Cancel</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import {
  DocumentBlockType,
  isClauseBlock,
  isApprovedTemplateBlock,
  TemplateType,
  type ClauseBlock,
} from '@template-repository/models/contract-templace'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import BlockPaletteItem from './document-block/BlockPaletteItem.vue'
import {
  parseSegments,
  getPlaceholderLabelFromConditions,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import ApprovedSubTemplatePicker from '@template-repository/components/builder-editor/preview/ApprovedSubTemplatePicker.vue'

const draftStore = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { addBlockModalContext } = storeToRefs(uiStore)
const { documentBlocks, semanticConditions, subTemplateSnapshots } = storeToRefs(draftStore)

const isContractWorkflow = computed(() => uiStore.workflow === 'contract')
const paletteBlockTypes = [
  { blockType: DocumentBlockType.Section, label: 'Section' },
  { blockType: DocumentBlockType.Text, label: 'Text' },
] as const

const isFrameContract = computed(() => draftStore.templateType === TemplateType.frameContract)

// For each template did, number of ApprovedTemplate blocks in the outline that reference it.
const referenceCountByDid = computed(() => {
  const inOutline = draftStore.blockIdsInOutline
  const count: Record<string, number> = {}
  for (const b of documentBlocks.value) {
    if (!isApprovedTemplateBlock(b) || !inOutline.has(b.blockId)) continue
    count[b.templateId] = (count[b.templateId] ?? 0) + 1
  }
  return count
})

/** Clause blocks that are not referenced in the document outline, sorted by title. */
const unusedClauses = computed((): ClauseBlock[] => {
  const inOutline = draftStore.blockIdsInOutline
  const clauses = documentBlocks.value.filter((b): b is ClauseBlock => isClauseBlock(b))
  const unused = clauses.filter((c) => !inOutline.has(c.blockId))
  return [...unused].sort((a, b) => (a.title ?? '').localeCompare(b.title ?? ''))
})

function getSegments(clause: ClauseBlock): Segment[] {
  return parseSegments(clause.text ?? '', semanticConditions.value)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, semanticConditions.value)
}

function handleCancel() {
  uiStore.closeAddBlockModal()
}

function handleAddBlock(blockType: DocumentBlockType) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, { blockType, text: '' })
  uiStore.closeAddBlockModal()
}

function handleAddApprovedTemplate(template: SubTemplateSnapshot) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: DocumentBlockType.ApprovedTemplate,
    text: '',
    templateId: template.did,
    version: template.version,
    document_number: template.document_number,
  })
  uiStore.closeAddBlockModal()
}

function handleAddClause(clauseBlockId: string) {
  const ctx = addBlockModalContext.value
  if (ctx === null) return
  draftStore.addBlock(ctx.parentBlockId, ctx.insertIndex, {
    blockType: DocumentBlockType.Clause,
    // Don't set text here, clauseBlockId is enough to link to the document outline.
    text: '',
    clauseBlockId,
  })
  uiStore.closeAddBlockModal()
}
</script>
