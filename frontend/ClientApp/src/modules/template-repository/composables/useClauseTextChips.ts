import type { Ref } from 'vue'
import type { SemanticCondition } from '@/modules/template-repository/models/contract-template'
import type { ClausePlaceholderHighlight } from '@template-repository/models/template-editor-ui-store'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'
import type { DcsContentSegment, DcsPlaceholder } from '@/models/dcs-jsonld'

export type Segment =
  | { type: 'text'; value: string }
  | { type: 'placeholder'; conditionId: string; parameterName: string; displayText: string }
  | { type: 'newline' }

export function isText(seg: Segment): seg is Extract<Segment, { type: 'text' }> {
  return seg.type === 'text'
}

export function isPlaceholder(seg: Segment): seg is Extract<Segment, { type: 'placeholder' }> {
  return seg.type === 'placeholder'
}

export function isNewline(seg: Segment): seg is Extract<Segment, { type: 'newline' }> {
  return seg.type === 'newline'
}

export const CHIP_HIGHLIGHT_CLASS = 'clause-chip-highlight'

const PLACEHOLDER_REGEX = /\{\{([^}]+)\}\}/g
const NEWLINE = '\n'

function toPlaceholderString(conditionId: string, parameterName: string): string {
  return `{{${conditionId}.${parameterName}}}`
}

function matchHighlight(
  conditionId: string,
  parameterName: string,
  h: NonNullable<ClausePlaceholderHighlight>,
): boolean {
  if (h.conditionId !== conditionId) return false
  if (h.parameterName != null) return h.parameterName === parameterName
  return true
}

/**
 * Splits clause text into text, placeholder, and newline segments.
 * Resolves {{conditionId.parameterName}} via conditions for displayText.
 * @example
 * parseSegments('From {{c1.start}} to {{c1.end}}.\n', conditions)
 * // => [
 * //   { type: 'text', value: 'From ' },
 * //   { type: 'placeholder', conditionId: 'c1', parameterName: 'start', displayText: 'start (Validity)' },
 * //   { type: 'text', value: ' to ' },
 * //   { type: 'placeholder', conditionId: 'c1', parameterName: 'end', displayText: 'end (Validity)' },
 * //   { type: 'text', value: '.' },
 * //   { type: 'newline' }
 * // ]
 */
export function parseSegments(text: string, conditions: SemanticCondition[]): Segment[] {
  const base = parsePlaceholders(text, conditions)
  return splitNewlines(base)
}

function parsePlaceholders(text: string, conditions: SemanticCondition[]): Segment[] {
  const segments: Segment[] = []
  let lastEnd = 0
  let m: RegExpExecArray | null
  PLACEHOLDER_REGEX.lastIndex = 0
  while ((m = PLACEHOLDER_REGEX.exec(text)) !== null) {
    if (m.index > lastEnd) {
      segments.push({ type: 'text', value: text.slice(lastEnd, m.index) })
    }
    const inner = m[1] ?? ''
    const dot = inner.indexOf('.')
    const conditionId = dot >= 0 ? inner.slice(0, dot) : inner
    const parameterName = dot >= 0 ? inner.slice(dot + 1) : ''
    const cond = conditions.find((c) => c.conditionId === conditionId)
    const conditionName = cond?.conditionName ?? conditionId
    const param = cond?.parameters.find((p) => p.parameterName === parameterName)
    const label = param ? semanticParameterLabel(param) : parameterName
    segments.push({
      type: 'placeholder',
      conditionId,
      parameterName,
      displayText: `${label} (${conditionName})`,
    })
    lastEnd = m.index + m[0].length
  }
  if (lastEnd < text.length) {
    segments.push({ type: 'text', value: text.slice(lastEnd) })
  }
  return segments
}

function splitNewlines(segments: Segment[]): Segment[] {
  const withNewlines: Segment[] = []
  for (const seg of segments) {
    if (isText(seg)) {
      const parts = seg.value.split(NEWLINE)
      for (let i = 0; i < parts.length; i++) {
        const part = parts[i]
        if (part) withNewlines.push({ type: 'text', value: part })
        if (i < parts.length - 1) withNewlines.push({ type: 'newline' })
      }
    } else {
      withNewlines.push(seg)
    }
  }
  return withNewlines
}

/**
 * Returns the set of conditionIds that appear in text (from {{conditionId.parameterName}} placeholders).
 * @example
 * conditionIdsInText('From {{c1.start}} to {{c1.end}}.')  // => new Set(['c1'])
 * conditionIdsInText('{{c1.a}} and {{c2.b}}')             // => new Set(['c1', 'c2'])
 */
export function conditionIdsInText(text: string): Set<string> {
  const set = new Set<string>()
  let m: RegExpExecArray | null
  PLACEHOLDER_REGEX.lastIndex = 0
  while ((m = PLACEHOLDER_REGEX.exec(text)) !== null) {
    const inner = m[1] ?? ''
    const dot = inner.indexOf('.')
    const conditionId = dot >= 0 ? inner.slice(0, dot) : inner
    if (conditionId) set.add(conditionId)
  }
  return set
}

// ---- JSON-LD DcsContentSegment[] helpers ----

function resolveFieldId(
  fieldId: string,
  conditions: SemanticCondition[],
): { conditionId: string; parameterName: string; displayText: string } | undefined {
  for (const cond of conditions) {
    const param = cond.parameters.find((p) => p.fieldId === fieldId)
    if (param) {
      return {
        conditionId: cond.conditionId,
        parameterName: param.parameterName,
        displayText: `${semanticParameterLabel(param)} (${cond.conditionName})`,
      }
    }
  }
  return undefined
}

/**
 * Converts JSON-LD DcsContentSegment[] into UI Segment[].
 * Resolves DcsPlaceholder.dcs:bindsTo['@id'] (fieldId) via semanticConditions.
 */
export function parseSegmentsFromContent(content: DcsContentSegment[], conditions: SemanticCondition[]): Segment[] {
  const segments: Segment[] = []
  for (const seg of content) {
    if (typeof seg === 'string') {
      const parts = seg.split('\n')
      for (let i = 0; i < parts.length; i++) {
        const part = parts[i]
        if (part) segments.push({ type: 'text', value: part })
        if (i < parts.length - 1) segments.push({ type: 'newline' })
      }
    } else {
      const fieldId = seg['dcs:bindsTo']['@id']
      const resolved = resolveFieldId(fieldId, conditions)
      segments.push({
        type: 'placeholder',
        conditionId: resolved?.conditionId ?? '',
        parameterName: resolved?.parameterName ?? '',
        displayText: resolved?.displayText ?? fieldId,
      })
    }
  }
  return segments
}

/**
 * Converts DcsContentSegment[] to the internal `{{conditionId.parameterName}}` string.
 * Used by the clause editor for DOM cursor arithmetic.
 */
export function contentToString(content: DcsContentSegment[], conditions: SemanticCondition[]): string {
  let result = ''
  for (const seg of content) {
    if (typeof seg === 'string') {
      result += seg
    } else {
      const fieldId = seg['dcs:bindsTo']['@id']
      const resolved = resolveFieldId(fieldId, conditions)
      if (resolved) {
        result += `{{${resolved.conditionId}.${resolved.parameterName}}}`
      }
    }
  }
  return result
}

/**
 * Converts an internal `{{conditionId.parameterName}}` string back to DcsContentSegment[].
 * Used by the clause editor when emitting modelValue.
 */
export function stringToContent(text: string, conditions: SemanticCondition[]): DcsContentSegment[] {
  const fieldIdMap = new Map<string, string>()
  for (const cond of conditions) {
    for (const param of cond.parameters) {
      if (param.fieldId) fieldIdMap.set(`${cond.conditionId}.${param.parameterName}`, param.fieldId)
    }
  }
  const content: DcsContentSegment[] = []
  const re = /\{\{([^}]+)\}\}/g
  let lastEnd = 0
  let m: RegExpExecArray | null
  re.lastIndex = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > lastEnd) content.push(text.slice(lastEnd, m.index))
    const key = m[1] ?? ''
    const fieldId = fieldIdMap.get(key)
    if (fieldId) {
      const placeholder: DcsPlaceholder = { '@type': 'dcs:Placeholder', 'dcs:bindsTo': { '@id': fieldId } }
      content.push(placeholder)
    } else {
      content.push(m[0])
    }
    lastEnd = m.index + m[0].length
  }
  if (lastEnd < text.length) content.push(text.slice(lastEnd))
  return content
}

/**
 * Returns the set of conditionIds referenced in a DcsContentSegment[] via dcs:bindsTo fieldIds.
 */
export function conditionIdsInContent(content: DcsContentSegment[], conditions: SemanticCondition[]): Set<string> {
  const set = new Set<string>()
  for (const seg of content) {
    if (typeof seg === 'string') continue
    const fieldId = seg['dcs:bindsTo']['@id']
    for (const cond of conditions) {
      if (cond.parameters.some((p) => p.fieldId === fieldId)) {
        set.add(cond.conditionId)
        break
      }
    }
  }
  return set
}

/**
 * Returns the conditionId.parameterName key set for all placeholders in content.
 * Used to track which parameter slots are already filled.
 */
export function usedPlaceholderKeysInContent(
  content: DcsContentSegment[],
  conditions: SemanticCondition[],
): Set<string> {
  const set = new Set<string>()
  for (const seg of content) {
    if (typeof seg === 'string') continue
    const fieldId = seg['dcs:bindsTo']['@id']
    for (const cond of conditions) {
      const param = cond.parameters.find((p) => p.fieldId === fieldId)
      if (param) {
        set.add(`${cond.conditionId}.${param.parameterName}`)
        break
      }
    }
  }
  return set
}

// ---- String-based helpers (public, kept for backwards compat with parseSegments callers) ----

/** Builds placeholder label from conditions. */
export function getPlaceholderLabelFromConditions(seg: Segment, conditions: SemanticCondition[]): string {
  if (!isPlaceholder(seg)) return ''
  const cond = conditions.find((c) => c.conditionId === seg.conditionId)
  const param = cond?.parameters.find((p) => p.parameterName === seg.parameterName)
  return param ? semanticParameterLabel(param) : seg.parameterName
}

export function useClauseTextChips(
  editorRef: Ref<HTMLDivElement | null>,
  highlight: Ref<ClausePlaceholderHighlight>,
  isMounted: Ref<boolean>,
) {
  /**
   * From the editor DOM, generates the clause template text using element info: text nodes → plain text;
   * chip spans (data-condition-id, data-parameter-name) → {{conditionId.parameterName}}.
   * @example
   * // editorEl children: [text "From ", span[data-condition-id=c1, data-parameter-name=start], text " to end."] → "From {{c1.start}} to end."
   */
  function getTemplateText(): string {
    const editorEl = editorRef.value
    if (!editorEl) return ''
    /** Accumulated clause string (text + {{id.param}} placeholders) */
    let result = ''
    function walk(node: Node) {
      if (node.nodeType === Node.TEXT_NODE) {
        result += node.textContent ?? ''
        return
      }
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as HTMLElement
        if (isPlaceholderElement(el)) {
          result += toPlaceholderString(el.dataset.conditionId, el.dataset.parameterName)
          return
        }
        if (isLineBreakElement(el)) {
          result += NEWLINE
          return
        }
      }
      node.childNodes.forEach(walk)
    }
    editorEl.childNodes.forEach(walk)
    return result
  }

  /** Logical length of a node: text length, placeholder length, or sum of children. */
  function getNodeLength(node: Node): number {
    if (node.nodeType === Node.TEXT_NODE) return (node.textContent ?? '').length
    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as HTMLElement
      if (isPlaceholderElement(el)) return toPlaceholderString(el.dataset.conditionId, el.dataset.parameterName).length
      if (isLineBreakElement(el)) return 1
    }
    let len = 0
    node.childNodes.forEach((child) => {
      len += getNodeLength(child)
    })
    return len
  }

  function computeLogicalOffsetInContainer(container: Node, targetNode: Node, targetOffset: number): number {
    let index = 0
    function walk(node: Node): boolean {
      if (node === targetNode) {
        index += targetOffset
        return true
      }
      if (node.nodeType === Node.TEXT_NODE) {
        index += getNodeLength(node)
        return false
      }
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as HTMLElement
        if (isPlaceholderElement(el) || isLineBreakElement(el)) {
          index += getNodeLength(node)
          return false
        }
      }
      for (let i = 0; i < node.childNodes.length; i++) {
        if (walk(node.childNodes.item(i))) return true
      }
      return false
    }
    walk(container)
    return index
  }

  /**
   * Returns the caret index in the logical clause string (same as getTemplateText() length units).
   * Text nodes and {{id.param}} each count as their string length.
   * @example
   * // Editor "From |{{c1.start}}." (| = caret) → getCursorIndex() returns 5
   */
  function getCursorIndex(): number {
    const editorEl = editorRef.value
    if (!editorEl) return 0
    const sel = document.getSelection()
    if (!sel || sel.rangeCount === 0) return 0
    if (!editorEl.contains(sel.anchorNode)) return 0
    const anchorNode = sel.anchorNode
    const anchorOffset = sel.anchorOffset
    if (!anchorNode) return 0
    // Caret is "after" the last child: anchorOffset = number of children before caret. Sum their logical lengths.
    if (anchorNode === editorEl) {
      let index = 0
      for (let i = 0; i < anchorOffset && i < editorEl.childNodes.length; i++) {
        const child = editorEl.childNodes.item(i)
        if (child) index += getNodeLength(child)
      }
      return index
    }
    // Caret is inside a descendant (text or element).
    return computeLogicalOffsetInContainer(editorEl, anchorNode, anchorOffset)
  }

  /** Returns selection range in logical template text indices.
   * If no selection, start === end (cursor).
   */
  function getSelectionRange(): { start: number; end: number } {
    const editorEl = editorRef.value
    if (!editorEl) return { start: 0, end: 0 }
    const sel = document.getSelection()
    if (!sel || sel.rangeCount === 0) return { start: 0, end: 0 }
    const anchorPos = indexOfNode(editorEl, sel.anchorNode, sel.anchorOffset)
    const focusPos = indexOfNode(editorEl, sel.focusNode, sel.focusOffset)
    if (anchorPos < 0 || focusPos < 0) return { start: 0, end: 0 }
    return { start: Math.min(anchorPos, focusPos), end: Math.max(anchorPos, focusPos) }
  }

  function indexOfNode(container: Node, targetNode: Node | null, targetOffset: number): number {
    if (!targetNode || !container.contains(targetNode)) return -1
    if (targetNode === container) {
      let index = 0
      for (let i = 0; i < targetOffset && i < container.childNodes.length; i++) {
        const child = container.childNodes.item(i)
        if (child) index += getNodeLength(child)
      }
      return index
    }
    return computeLogicalOffsetInContainer(container, targetNode, targetOffset)
  }

  function setCursorAfter(node: Node): void {
    const sel = document.getSelection()
    if (!sel) return
    const range = document.createRange()
    range.setStartAfter(node)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  }

  function setCursorAt(editorEl: Node, targetOffset: number): void {
    const sel = document.getSelection()
    if (!sel) return
    const selection: Selection = sel
    const range = document.createRange()
    let offset = 0
    function walk(node: Node): boolean {
      if (node.nodeType === Node.TEXT_NODE) {
        const len = (node.textContent ?? '').length
        if (offset + len >= targetOffset) {
          range.setStart(node, Math.min(targetOffset - offset, len))
          range.collapse(true)
          selection.removeAllRanges()
          selection.addRange(range)
          return true
        }
        offset += len
        return false
      }
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as HTMLElement
        if (isPlaceholderElement(el)) {
          const len = toPlaceholderString(el.dataset.conditionId, el.dataset.parameterName).length
          if (offset + len >= targetOffset) {
            range.setStartAfter(node)
            range.collapse(true)
            selection.removeAllRanges()
            selection.addRange(range)
            return true
          }
          offset += len
          return false
        }
        if (isLineBreakElement(el)) {
          const len = 1
          if (offset + len >= targetOffset) {
            range.setStartAfter(node)
            range.collapse(true)
            selection.removeAllRanges()
            selection.addRange(range)
            return true
          }
          offset += len
          return false
        }
      }
      for (let i = 0; i < node.childNodes.length; i++) {
        const child = node.childNodes.item(i)
        if (walk(child)) return true
      }
      return false
    }
    if (!walk(editorEl) && offset < targetOffset) {
      range.selectNodeContents(editorEl)
      range.collapse(false)
      selection.removeAllRanges()
      selection.addRange(range)
    }
  }

  /**
   * Rebuilds the editor DOM from the clause template text: text as text nodes, placeholders as
   * non-editable chip spans. Applies current highlight to matching chips.
   * @example
   * syncFromTemplateText('From {{c1.start}} to {{c1.end}}.', conditions)
   */
  function syncFromTemplateText(templateText: string, conditions: SemanticCondition[]): void {
    if (!isMounted.value) return
    const el = editorRef.value
    if (!el) return
    const segments = parseSegments(templateText, conditions)
    const h = highlight.value
    el.replaceChildren()
    const baseClass =
      'inline-flex items-center px-2 py-0.5 rounded text-primary bg-primary/10 border-0 border-b border-neutral/70 text-xs font-medium align-baseline cursor-pointer'
    for (const seg of segments) {
      if (isText(seg)) {
        el.appendChild(document.createTextNode(seg.value))
      } else if (seg.type === 'newline') {
        const br = document.createElement('br')
        br.dataset.line = 'true'
        el.appendChild(br)
      } else {
        // Chip span for placeholder
        const span = document.createElement('span')
        span.contentEditable = 'false'
        span.dataset.conditionId = seg.conditionId
        span.dataset.parameterName = seg.parameterName
        span.textContent = seg.displayText
        const highlightClass =
          h && matchHighlight(seg.conditionId, seg.parameterName, h) ? ` ${CHIP_HIGHLIGHT_CLASS}` : ''
        span.className = baseClass + highlightClass
        el.appendChild(span)
      }
    }
    applyHighlight()
  }

  function applyHighlight(): void {
    if (!isMounted.value) return
    const el = editorRef.value
    if (!el) return
    const h = highlight.value
    el.querySelectorAll('[data-condition-id][data-parameter-name]').forEach((node) => {
      const span = node as HTMLElement
      const cid = span.getAttribute('data-condition-id') ?? ''
      const pname = span.getAttribute('data-parameter-name') ?? ''
      const match = h && matchHighlight(cid, pname, h)
      if (match) {
        span.classList.add(CHIP_HIGHLIGHT_CLASS)
      } else {
        span.classList.remove(CHIP_HIGHLIGHT_CLASS)
      }
    })
  }

  /**
   * Handles paste: preventDefault, reads clipboard plain text, replaces
   * selection (or insert at cursor) in current template text. Returns
   * new value and cursor position; caller must emit, sync DOM, and set cursor.
   */
  function handlePaste(e: ClipboardEvent): { newValue: string; newCursorPos: number } {
    e.preventDefault()
    const plain = e.clipboardData?.getData('text/plain') ?? ''
    const current = getTemplateText()
    const { start, end } = getSelectionRange()
    const newValue = current.slice(0, start) + plain + current.slice(end)
    const newCursorPos = start + plain.length
    return { newValue, newCursorPos }
  }

  /** Inserts a logical newline at the current selection and returns new value and cursor position. */
  function insertNewlineAtSelection(): { newValue: string; newCursorPos: number } {
    const current = getTemplateText()
    const { start, end } = getSelectionRange()
    const before = current.slice(0, start)
    const after = current.slice(end)
    const newValue = before + NEWLINE + after
    const newCursorPos = start + 1
    return { newValue, newCursorPos }
  }

  /** Add space before/after insert unless already space or period. Returns new full value and length of inserted part (for cursor). */
  function wrapSpaces(before: string, insert: string, after: string): { value: string; insertLength: number } {
    const needBefore = before.length > 0 && !before.endsWith(' ') && !before.endsWith('。')
    const needAfter = after.length > 0 && !after.startsWith(' ')
    const value = before + (needBefore ? ' ' : '') + insert + (needAfter ? ' ' : '') + after
    const insertLength = (needBefore ? 1 : 0) + insert.length + (needAfter ? 1 : 0)
    return { value, insertLength }
  }

  /** Placeholder chip span: <span contenteditable="false" data-condition-id="c1" data-parameter-name="start"> */
  function isPlaceholderElement(
    el: HTMLElement,
  ): el is HTMLElement & { dataset: DOMStringMap & { conditionId: string; parameterName: string } } {
    return el.dataset.conditionId != null && el.dataset.parameterName != null
  }
  /** Logical newline: <br data-line="true"> */
  function isLineBreakElement(el: HTMLElement): el is HTMLElement & { dataset: DOMStringMap & { line: string } } {
    return el.dataset.line === 'true'
  }

  return {
    parseSegments,
    getTemplateText,
    getCursorIndex,
    handlePaste,
    insertNewlineAtSelection,
    setCursorAfter,
    setCursorAt,
    syncFromTemplateText,
    applyHighlight,
    wrapSpaces,
  }
}
