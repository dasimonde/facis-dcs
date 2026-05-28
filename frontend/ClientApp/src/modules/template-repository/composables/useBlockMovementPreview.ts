import { computed, ref, unref, onBeforeUnmount, type MaybeRef, type Component } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import type { DocumentOutline } from '@template-repository/models/contract-templace'
import IconMoveLeft from '@template-repository/components/builder-editor/toolbar/icons/IconMoveLeft.vue'
import IconMoveRight from '@template-repository/components/builder-editor/toolbar/icons/IconMoveRight.vue'

const VERTICAL_ENTER_MS = 100
const VERTICAL_LEAVE_MS = 120
const HORIZONTAL_ENTER_MS = 100
const HORIZONTAL_LEAVE_MS = 120
const INDENT_PER_LEVEL = 16

function indentWidth(depth: number): string {
  return `${depth * INDENT_PER_LEVEL}px`
}

function collectDescendantBlockIds(outline: DocumentOutline, blockId: string): Set<string> {
  const set = new Set<string>()
  const block = outline.find((b) => b.blockId === blockId)
  const childIds = block?.children ?? []
  for (const id of childIds) {
    set.add(id)
    collectDescendantBlockIds(outline, id).forEach((desc) => set.add(desc))
  }
  return set
}

export interface BlockMovementPreviewToolbarContext {
  blockId: string
  prevSiblingBlockId?: string
  nextSiblingBlockId?: string
  canMoveUp: boolean
  canMoveDown: boolean
  canOutdent: boolean
  canIndent: boolean
}

export interface BlockMovementPreviewToolbarHandlers {
  onMoveUpEnter: () => void
  onMoveUpLeave: () => void
  onMoveDownEnter: () => void
  onMoveDownLeave: () => void
  onOutdentEnter: () => void
  onOutdentLeave: () => void
  onIndentEnter: () => void
  onIndentLeave: () => void
  clearVerticalPreview: () => void
  clearHorizontalPreview: () => void
}

/**
 * Composable for block movement hover preview (vertical swap + horizontal indent/outdent).
 * - Pass outline to get derived state for EditorBlocks (fade set, swap target, indent width, arrows).
 * - Call createToolbarHandlers(getContext) to get handlers for BlockToolbar (timers + set/clear preview).
 */
export function useBlockMovementPreview(outline?: MaybeRef<DocumentOutline>) {
  const uiStore = useTemplateEditorUiStore()
  const { blockMovementPreview } = storeToRefs(uiStore)

  const verticalFadeOutSet = outline
    ? computed(() => {
        const preview = blockMovementPreview.value
        if (preview?.type !== 'vertical') return new Set<string>()
        const outlineVal = unref(outline)
        const sourceDesc = collectDescendantBlockIds(outlineVal, preview.sourceBlockId)
        const targetDesc = collectDescendantBlockIds(outlineVal, preview.targetBlockId)
        return new Set([...sourceDesc, ...targetDesc])
      })
    : undefined

  function isInFadeOutSet(blockId: string): boolean {
    return verticalFadeOutSet?.value.has(blockId) ?? false
  }

  function isSwapPreviewTarget(blockId: string): boolean {
    const preview = blockMovementPreview.value
    return !!preview && preview.type === 'vertical' && preview.targetBlockId === blockId
  }

  function effectiveIndentWidth(item: { blockId: string; depthLevel: number }): string {
    const preview = blockMovementPreview.value
    if (preview?.type === 'horizontal' && preview.blockId === item.blockId) {
      const depth = preview.direction === 'left' ? item.depthLevel - 1 : item.depthLevel + 1
      return indentWidth(Math.max(0, depth))
    }
    return indentWidth(item.depthLevel)
  }

  function horizontalPreviewFor(item: { blockId: string }): boolean {
    const preview = blockMovementPreview.value
    return !!preview && preview.type === 'horizontal' && preview.blockId === item.blockId
  }

  function horizontalArrowIcon(item: { blockId: string }): Component {
    const preview = blockMovementPreview.value
    if (preview?.type !== 'horizontal' || preview.blockId !== item.blockId) return IconMoveLeft
    return preview.direction === 'left' ? IconMoveLeft : IconMoveRight
  }

  /**
   * Create toolbar hover/click handlers for one block.
   * Cleans up timers on unmount.
   */
  function createToolbarHandlers(
    getContext: () => BlockMovementPreviewToolbarContext,
  ): BlockMovementPreviewToolbarHandlers {
    const moveUpEnterTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const moveUpLeaveTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const moveDownEnterTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const moveDownLeaveTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const outdentEnterTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const outdentLeaveTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const indentEnterTimer = ref<ReturnType<typeof setTimeout> | null>(null)
    const indentLeaveTimer = ref<ReturnType<typeof setTimeout> | null>(null)

    function clearVerticalPreview() {
      uiStore.setBlockMovementPreview(null)
    }

    function clearHorizontalPreview() {
      uiStore.setBlockMovementPreview(null)
    }

    function onMoveUpEnter() {
      const ctx = getContext()
      if (!ctx.canMoveUp || !ctx.prevSiblingBlockId) return
      if (moveUpLeaveTimer.value) {
        clearTimeout(moveUpLeaveTimer.value)
        moveUpLeaveTimer.value = null
      }
      moveUpEnterTimer.value = setTimeout(() => {
        moveUpEnterTimer.value = null
        uiStore.setBlockMovementPreview({
          type: 'vertical',
          sourceBlockId: ctx.blockId,
          targetBlockId: ctx.prevSiblingBlockId!,
        })
      }, VERTICAL_ENTER_MS)
    }

    function onMoveUpLeave() {
      if (moveUpEnterTimer.value) {
        clearTimeout(moveUpEnterTimer.value)
        moveUpEnterTimer.value = null
      }
      moveUpLeaveTimer.value = setTimeout(() => {
        moveUpLeaveTimer.value = null
        clearVerticalPreview()
      }, VERTICAL_LEAVE_MS)
    }

    function onMoveDownEnter() {
      const ctx = getContext()
      if (!ctx.canMoveDown || !ctx.nextSiblingBlockId) return
      if (moveDownLeaveTimer.value) {
        clearTimeout(moveDownLeaveTimer.value)
        moveDownLeaveTimer.value = null
      }
      moveDownEnterTimer.value = setTimeout(() => {
        moveDownEnterTimer.value = null
        uiStore.setBlockMovementPreview({
          type: 'vertical',
          sourceBlockId: ctx.blockId,
          targetBlockId: ctx.nextSiblingBlockId!,
        })
      }, VERTICAL_ENTER_MS)
    }

    function onMoveDownLeave() {
      if (moveDownEnterTimer.value) {
        clearTimeout(moveDownEnterTimer.value)
        moveDownEnterTimer.value = null
      }
      moveDownLeaveTimer.value = setTimeout(() => {
        moveDownLeaveTimer.value = null
        clearVerticalPreview()
      }, VERTICAL_LEAVE_MS)
    }

    function onOutdentEnter() {
      if (outdentLeaveTimer.value) {
        clearTimeout(outdentLeaveTimer.value)
        outdentLeaveTimer.value = null
      }
      outdentEnterTimer.value = setTimeout(() => {
        outdentEnterTimer.value = null
        const ctx = getContext()
        uiStore.setBlockMovementPreview({ type: 'horizontal', blockId: ctx.blockId, direction: 'left' })
      }, HORIZONTAL_ENTER_MS)
    }

    function onOutdentLeave() {
      if (outdentEnterTimer.value) {
        clearTimeout(outdentEnterTimer.value)
        outdentEnterTimer.value = null
      }
      outdentLeaveTimer.value = setTimeout(() => {
        outdentLeaveTimer.value = null
        clearHorizontalPreview()
      }, HORIZONTAL_LEAVE_MS)
    }

    function onIndentEnter() {
      if (indentLeaveTimer.value) {
        clearTimeout(indentLeaveTimer.value)
        indentLeaveTimer.value = null
      }
      indentEnterTimer.value = setTimeout(() => {
        indentEnterTimer.value = null
        const ctx = getContext()
        uiStore.setBlockMovementPreview({ type: 'horizontal', blockId: ctx.blockId, direction: 'right' })
      }, HORIZONTAL_ENTER_MS)
    }

    function onIndentLeave() {
      if (indentEnterTimer.value) {
        clearTimeout(indentEnterTimer.value)
        indentEnterTimer.value = null
      }
      indentLeaveTimer.value = setTimeout(() => {
        indentLeaveTimer.value = null
        clearHorizontalPreview()
      }, HORIZONTAL_LEAVE_MS)
    }

    onBeforeUnmount(() => {
      ;[
        moveUpEnterTimer,
        moveUpLeaveTimer,
        moveDownEnterTimer,
        moveDownLeaveTimer,
        outdentEnterTimer,
        outdentLeaveTimer,
        indentEnterTimer,
        indentLeaveTimer,
      ].forEach((t) => {
        if (t.value) clearTimeout(t.value)
      })
    })

    return {
      onMoveUpEnter,
      onMoveUpLeave,
      onMoveDownEnter,
      onMoveDownLeave,
      onOutdentEnter,
      onOutdentLeave,
      onIndentEnter,
      onIndentLeave,
      clearVerticalPreview,
      clearHorizontalPreview,
    }
  }

  return {
    isInFadeOutSet,
    isSwapPreviewTarget,
    effectiveIndentWidth,
    horizontalPreviewFor,
    horizontalArrowIcon,
    createToolbarHandlers,
  }
}
