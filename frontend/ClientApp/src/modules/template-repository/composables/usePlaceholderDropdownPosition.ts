import { ref } from 'vue'
import type { Ref } from 'vue'

export type PlaceholderDropdownMode = 'anchor' | 'caret'

const ANCHOR_CLASS =
  'absolute left-0 right-0 mt-1 z-20 bg-base-100 border border-base-300 rounded-lg shadow-lg max-h-48 overflow-y-auto'
const CARET_BASE_CLASS = 'z-20 bg-base-100 border border-base-300 rounded-lg shadow-lg max-h-48 overflow-y-auto'

const CARET_OFFSET_PX = 4

/**
 * Encapsulates placeholder suggestions dropdown positioning so you can switch
 * between 'anchor' (below editor, current behavior) and 'caret' (below cursor).
 */
export function usePlaceholderDropdownPosition(editorRef: Ref<HTMLDivElement | null>, mode: PlaceholderDropdownMode) {
  const dropdownStyle = ref<Record<string, string>>({})
  const dropdownClass = mode === 'anchor' ? ANCHOR_CLASS : CARET_BASE_CLASS

  function updatePosition() {
    if (mode !== 'caret') return
    const el = editorRef.value
    const sel = document.getSelection()
    if (!el || !sel || sel.rangeCount === 0) return
    const range = sel.getRangeAt(0)
    const rect = range.getBoundingClientRect()
    dropdownStyle.value = {
      position: 'fixed',
      top: `${rect.bottom + CARET_OFFSET_PX}px`,
      left: `${rect.left}px`,
      minWidth: '12rem',
    }
  }

  function clearPosition() {
    if (mode === 'caret') dropdownStyle.value = {}
  }

  return { dropdownStyle, dropdownClass, updatePosition, clearPosition }
}
