<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import EditorBlock from '@template-repository/components/builder-editor/document-block/EditorBlock.vue'
import { useBlockMovementPreview } from '@template-repository/composables/useBlockMovementPreview'
import { type FlattenedOutlineItem, useFlattenedOutline } from '@template-repository/composables/useFlattenedOutline'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { isDcsSection } from '@/models/dcs-jsonld'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { EnrichedBlockItem } from '@template-repository/models/enriched-block-item'

const draftStore = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { layout, blocks } = storeToRefs(draftStore)
const { isInFadeOutSet, effectiveIndentWidth, horizontalPreviewFor, horizontalArrowIcon } =
  useBlockMovementPreview(layout)

const flattened = useFlattenedOutline(layout)

const flatItemsWithBlock = computed((): EnrichedBlockItem[] => {
  const layoutVal = layout.value
  const root = layoutVal.find((n) => n['dcs:isRoot'])
  const blockById = new Map<string, DcsBlock>(blocks.value.map((b) => [b['@id'], b]))
  return flattened.value.map((item) => enrichFlatItem(item, layoutVal, blockById, root))
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

function layoutNodeChildIds(node: DcsLayoutNode): string[] {
  return node['dcs:children']['@list'].map((r) => r['@id'])
}

/**
 * Enriches a flattened layout item with block data and outdent/indent params for the toolbar.
 */
function enrichFlatItem(
  item: FlattenedOutlineItem,
  layout: DcsLayoutNode[],
  blockById: Map<string, DcsBlock>,
  root: DcsLayoutNode | undefined,
): EnrichedBlockItem {
  const parentNode = layout.find((n) => n['@id'] === item.parentBlockId)
  const parentChildren = parentNode ? layoutNodeChildIds(parentNode) : []
  const siblingCount = parentChildren.length
  const isDirectChildOfRoot = !!root && item.parentBlockId === root['@id']
  const isLastChild = item.siblingIndex === siblingCount - 1
  const grandparentNode = layout.find((n) => layoutNodeChildIds(n).includes(item.parentBlockId))
  const parentIndexInGrandparent = grandparentNode
    ? layoutNodeChildIds(grandparentNode).indexOf(item.parentBlockId)
    : -1
  const canOutdent = !isDirectChildOfRoot && isLastChild
  const outdentGrandparentBlockId = grandparentNode?.['@id'] ?? ''
  const outdentInsertIndex = parentIndexInGrandparent + 1

  const prevSiblingBlockId = parentChildren[item.siblingIndex - 1]
  const nextSiblingBlockId = parentChildren[item.siblingIndex + 1]
  const prevSiblingBlock = prevSiblingBlockId ? blockById.get(prevSiblingBlockId) : undefined
  const prevSiblingIsContainer = !!prevSiblingBlock && isDcsSection(prevSiblingBlock)
  const canIndent = item.siblingIndex > 0 && prevSiblingIsContainer
  const prevSiblingOutlineNode = prevSiblingBlockId ? layout.find((n) => n['@id'] === prevSiblingBlockId) : undefined
  const indentInsertIndex = prevSiblingOutlineNode ? layoutNodeChildIds(prevSiblingOutlineNode).length : 0

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
  }
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div
      v-for="item in flatItemsWithBlock"
      :key="item.blockId"
      :class="[
        'flex min-w-0 items-stretch',
        'transition-opacity duration-200 ease-out',
        isInFadeOutSet(item.blockId) && 'opacity-50',
      ]"
    >
      <!-- Indent area: width by depth, left border for children to show hierarchy -->
      <div
        v-if="item.block"
        :class="[
          'relative flex min-h-10 shrink-0 items-center',
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
