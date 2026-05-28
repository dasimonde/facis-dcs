<template>
  <div class="flex flex-col gap-2">
    <div
      v-for="item in flatItemsWithBlock"
      :key="item.blockId"
      :class="[
        'flex min-w-0 items-stretch',
        'transition-[opacity] duration-200 ease-out',
        isInFadeOutSet(item.blockId) && 'opacity-50',
      ]"
    >
      <!-- Indent area: width by depth, left border for children to show hierarchy -->
      <div
        v-if="item.block && !isMergedApprovedTemplateBlock(item.block)"
        :class="[
          'relative flex min-h-[2.5rem] flex-shrink-0 items-center',
          'transition-[width] duration-300 ease-out',
          item.depthLevel > 0 && !horizontalPreviewFor(item) && 'border-l-2 border-base-300',
          horizontalPreviewFor(item) && 'border-l-2 border-primary',
        ]"
        :style="{ width: effectiveIndentWidth(item) }"
        aria-hidden
      >
        <component
          :is="horizontalArrowIcon(item)"
          v-if="horizontalPreviewFor(item)"
          :size="14"
          class="pointer-events-none absolute top-1/2 left-0.5 -translate-y-1/2 text-primary"
        />
      </div>
      <EditorBlock
        :item="item"
        @select="selectBlock(item.blockId)"
        @insert-above="onInsertAbove(item)"
        @insert-below="onInsertBelow(item)"
        @insert-nest="onInsertNest(item)"
        @confirm="(payload) => confirmBlock(item.blockId, payload)"
        @move-up="onMoveUp(item)"
        @move-down="onMoveDown(item)"
        @move-outdent="onMoveOutdent(item)"
        @move-indent="onMoveIndent(item)"
        @delete="deleteBlock(item.blockId)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { useFlattenedOutline, type FlattenedOutlineItem } from '@template-repository/composables/useFlattenedOutline'
import type {
  DocumentBlock,
  DocumentOutline,
  DocumentOutlineBlock,
  MergedApprovedTemplateBlock,
} from '@template-repository/models/contract-templace'
import type { EnrichedBlockItem } from '@template-repository/models/enriched-block-item'
import {
  isSectionBlock,
  isApprovedTemplateBlock,
  isMergedApprovedTemplateBlock,
} from '@template-repository/models/contract-templace'
import EditorBlock from '@template-repository/components/builder-editor/document-block/EditorBlock.vue'
import { useBlockMovementPreview } from '@template-repository/composables/useBlockMovementPreview'
import { getOwnerBlockIdFromMergedBlockId, isMergedBlockId } from '@template-repository/utils/template-data-ref'

const draftStore = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { documentOutline, documentBlocks } = storeToRefs(draftStore)
const { isInFadeOutSet, effectiveIndentWidth, horizontalPreviewFor, horizontalArrowIcon } =
  useBlockMovementPreview(documentOutline)

const flattened = useFlattenedOutline(documentOutline)

const flatItemsWithBlock = computed((): EnrichedBlockItem[] => {
  const outline = documentOutline.value
  const root = outline.find((b) => b.isRoot)
  const blockById = new Map(documentBlocks.value.map((b) => [b.blockId, b]))
  return flattened.value.map((item) => enrichFlatItem(item, outline, blockById, root))
})

function selectBlock(blockId: string) {
  uiStore.setSelectedBlockId(blockId)
}

function openAddBlockModal(parentBlockId: string, insertIndex: number) {
  uiStore.openAddBlockModal(parentBlockId, insertIndex)
}
function confirmBlock(blockId: string, payload: { title: string; text: string }) {
  selectBlock(blockId)
  draftStore.updateBlock(blockId, payload)
}
function onInsertAbove(item: { blockId: string; parentBlockId: string; siblingIndex: number }) {
  selectBlock(item.blockId)
  openAddBlockModal(item.parentBlockId, item.siblingIndex)
}
function onInsertBelow(item: { blockId: string; parentBlockId: string; siblingIndex: number }) {
  selectBlock(item.blockId)
  openAddBlockModal(item.parentBlockId, item.siblingIndex + 1)
}
function onInsertNest(item: { blockId: string }) {
  selectBlock(item.blockId)
  openAddBlockModal(item.blockId, 0)
}
function onMoveUp(item: { blockId: string; parentBlockId: string; siblingIndex: number }) {
  selectBlock(item.blockId)
  draftStore.moveBlock(item.blockId, item.parentBlockId, item.siblingIndex - 1)
}
function onMoveDown(item: { blockId: string; parentBlockId: string; siblingIndex: number }) {
  selectBlock(item.blockId)
  draftStore.moveBlock(item.blockId, item.parentBlockId, item.siblingIndex + 1)
}
function onMoveOutdent(item: { blockId: string; outdentGrandparentBlockId: string; outdentInsertIndex: number }) {
  if (!item.outdentGrandparentBlockId) return
  selectBlock(item.blockId)
  draftStore.moveBlock(item.blockId, item.outdentGrandparentBlockId, item.outdentInsertIndex)
}
function onMoveIndent(item: { blockId: string; indentParentBlockId: string; indentInsertIndex: number }) {
  if (!item.indentParentBlockId) return
  selectBlock(item.blockId)
  draftStore.moveBlock(item.blockId, item.indentParentBlockId, item.indentInsertIndex)
}
function deleteBlock(blockId: string) {
  draftStore.deleteBlock(blockId)
  uiStore.setSelectedBlockId(null)
}

/**
 * Enriches a flattened outline item with block data and outdent/indent params for the toolbar.
 */
function enrichFlatItem(
  item: FlattenedOutlineItem,
  outline: DocumentOutline,
  blockById: Map<string, DocumentBlock>,
  root: DocumentOutlineBlock | undefined,
): EnrichedBlockItem {
  const parentNode = outline.find((b) => b.blockId === item.parentBlockId)
  const siblingCount = parentNode?.children?.length ?? 0
  const isDirectChildOfRoot = !!root && item.parentBlockId === root.blockId
  const isLastChild = item.siblingIndex === siblingCount - 1
  const grandparentNode = outline.find((b) => b.children?.includes(item.parentBlockId))
  const parentIndexInGrandparent = grandparentNode?.children?.indexOf(item.parentBlockId) ?? -1
  const canOutdent = !isDirectChildOfRoot && isLastChild
  const outdentGrandparentBlockId = grandparentNode?.blockId ?? ''
  const outdentInsertIndex = parentIndexInGrandparent + 1

  const prevSiblingBlockId = parentNode?.children?.[item.siblingIndex - 1]
  const nextSiblingBlockId = parentNode?.children?.[item.siblingIndex + 1]
  const prevSiblingBlock = prevSiblingBlockId ? blockById.get(prevSiblingBlockId) : undefined
  const prevSiblingIsContainer =
    !!prevSiblingBlock && (isSectionBlock(prevSiblingBlock) || isApprovedTemplateBlock(prevSiblingBlock))
  const canIndent = item.siblingIndex > 0 && prevSiblingIsContainer
  const prevSiblingOutlineNode = prevSiblingBlockId ? outline.find((b) => b.blockId === prevSiblingBlockId) : undefined
  const indentInsertIndex = prevSiblingOutlineNode?.children?.length ?? 0
  let mergedApprovedBlock: MergedApprovedTemplateBlock | undefined = undefined
  if (isMergedBlockId(item.blockId)) {
    const ownerBlockId = getOwnerBlockIdFromMergedBlockId(item.blockId)
    mergedApprovedBlock = documentBlocks.value.find(
      (b) => b.blockId === ownerBlockId && isMergedApprovedTemplateBlock(b),
    ) as MergedApprovedTemplateBlock | undefined
  }

  return {
    blockId: item.blockId,
    block: blockById.get(item.blockId),
    siblingIndex: item.siblingIndex,
    siblingCount,
    parentBlockId: item.parentBlockId,
    depthLevel: item.depthLevel,
    prevSiblingBlockId: prevSiblingBlockId ?? undefined,
    nextSiblingBlockId: nextSiblingBlockId ?? undefined,
    canOutdent,
    canIndent,
    outdentGrandparentBlockId,
    outdentInsertIndex,
    indentParentBlockId: prevSiblingBlockId ?? '',
    indentInsertIndex,
    mergedApprovedBlock,
  }
}
</script>
