<template>
  <div class="flex shrink-0 flex-col items-start gap-1" role="toolbar" aria-label="Block actions">
    <div class="flex items-center gap-0.5">
      <button
        type="button"
        :class="btnIcon"
        title="Insert above"
        aria-label="Insert block above"
        @click="onInsertAbove"
      >
        <IconInsertAbove :size="20" class="h-5 w-5" />
      </button>
      <button
        type="button"
        :class="btnIcon"
        title="Insert below"
        aria-label="Insert block below"
        @click="onInsertBelow"
      >
        <IconInsertBelow :size="20" class="h-5 w-5" />
      </button>
      <button
        v-if="canInsertNest"
        type="button"
        :class="btnIcon"
        title="Insert nested"
        aria-label="Insert block nested"
        @click="onInsertNest"
      >
        <IconInsertNestBelow :size="20" class="h-5 w-5" />
      </button>
      <button
        type="button"
        :class="[btnIcon, !canMoveUp && 'cursor-not-allowed opacity-50']"
        title="Move up"
        aria-label="Move up"
        :disabled="!canMoveUp"
        @click="onMoveUp"
        @pointerenter="preview.onMoveUpEnter"
        @pointerleave="preview.onMoveUpLeave"
      >
        <IconMoveUp :size="20" class="h-5 w-5" />
      </button>
      <button
        type="button"
        :class="[btnIcon, !canMoveDown && 'cursor-not-allowed opacity-50']"
        title="Move down"
        aria-label="Move down"
        :disabled="!canMoveDown"
        @click="onMoveDown"
        @pointerenter="preview.onMoveDownEnter"
        @pointerleave="preview.onMoveDownLeave"
      >
        <IconMoveDown :size="20" class="h-5 w-5" />
      </button>
      <button
        v-if="canOutdent"
        type="button"
        :class="btnIcon"
        title="Outdent (move to same level as parent)"
        aria-label="Outdent"
        @click="onMoveOutdent"
        @mouseenter="preview.onOutdentEnter"
        @mouseleave="preview.onOutdentLeave"
      >
        <IconMoveLeft :size="20" class="h-5 w-5" />
      </button>
      <button
        v-if="canIndent"
        type="button"
        :class="btnIcon"
        title="Indent (move into block above)"
        aria-label="Indent"
        @click="onMoveIndent"
        @mouseenter="preview.onIndentEnter"
        @mouseleave="preview.onIndentLeave"
      >
        <IconMoveRight :size="20" class="h-5 w-5" />
      </button>
      <button
        type="button"
        :class="[btnIcon, 'text-error hover:bg-error/10']"
        title="Delete"
        aria-label="Delete block"
        @click="onDelete"
      >
        <IconTrash :size="20" class="h-5 w-5" />
      </button>
    </div>
    <div v-if="isDirty" class="flex w-full items-center gap-1">
      <button type="button" class="btn flex-1 btn-outline btn-xs" @click="onCancel">Cancel</button>
      <button type="button" class="btn flex-1 btn-xs btn-primary" @click="onConfirm">Confirm</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { EnrichedBlockItem } from '@template-repository/models/enriched-block-item'
import { isApprovedTemplateBlock, isSectionBlock } from '@template-repository/models/contract-templace'
import { useBlockMovementPreview } from '@template-repository/composables/useBlockMovementPreview'
import IconInsertAbove from './icons/IconInsertAbove.vue'
import IconInsertBelow from './icons/IconInsertBelow.vue'
import IconInsertNestBelow from './icons/IconInsertNestBelow.vue'
import IconTrash from './icons/IconTrash.vue'
import IconMoveUp from './icons/IconMoveUp.vue'
import IconMoveDown from './icons/IconMoveDown.vue'
import IconMoveLeft from './icons/IconMoveLeft.vue'
import IconMoveRight from './icons/IconMoveRight.vue'

const btnIcon = 'btn btn-ghost btn-xs btn-square'

const props = withDefaults(
  defineProps<{
    item: EnrichedBlockItem
    isDirty?: boolean
  }>(),
  { isDirty: false },
)

const emit = defineEmits<{
  insertAbove: []
  insertBelow: []
  insertNest: []
  confirm: []
  cancel: []
  moveUp: []
  moveDown: []
  moveOutdent: []
  moveIndent: []
  delete: []
}>()

const canInsertNest = computed(
  () => !!props.item.block && (isSectionBlock(props.item.block) || isApprovedTemplateBlock(props.item.block)),
)
const canMoveUp = computed(() => props.item.siblingIndex > 0)
const canMoveDown = computed(() => props.item.siblingIndex < props.item.siblingCount - 1)
const canOutdent = computed(() => props.item.canOutdent)
const canIndent = computed(() => props.item.canIndent)

const preview = useBlockMovementPreview().createToolbarHandlers(() => ({
  blockId: props.item.blockId,
  prevSiblingBlockId: props.item.prevSiblingBlockId,
  nextSiblingBlockId: props.item.nextSiblingBlockId,
  canMoveUp: canMoveUp.value,
  canMoveDown: canMoveDown.value,
  canOutdent: props.item.canOutdent,
  canIndent: props.item.canIndent,
}))

function onInsertAbove() {
  emit('insertAbove')
}
function onInsertBelow() {
  emit('insertBelow')
}
function onInsertNest() {
  emit('insertNest')
}
function onConfirm() {
  emit('confirm')
}
function onCancel() {
  emit('cancel')
}
function onMoveUp() {
  preview.clearVerticalPreview()
  emit('moveUp')
}
function onMoveDown() {
  preview.clearVerticalPreview()
  emit('moveDown')
}
function onMoveOutdent() {
  preview.clearHorizontalPreview()
  emit('moveOutdent')
}
function onMoveIndent() {
  preview.clearHorizontalPreview()
  emit('moveIndent')
}
function onDelete() {
  emit('delete')
}
</script>
