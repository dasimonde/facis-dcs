<template>
  <div class="clause-text-editor space-y-4">
    <div class="relative">
      <div
        ref="editorRef"
        class="clause-editor textarea-bordered textarea min-h-24 w-full text-sm textarea-sm wrap-break-word whitespace-pre-wrap"
        contenteditable="true"
        data-placeholder=""
        @input="onEditorInput"
        @paste="onEditorPaste"
        @blur="onEditorBlur"
        @keydown="onEditorKeydown"
        @click="onEditorClick"
      ></div>
      <!-- placeholder suggestions -->
      <div v-show="showPlaceholderSuggestions" :style="placeholderDropdownStyle" :class="placeholderDropdownClass">
        <p class="border-b border-base-200 px-3 py-2 text-xs text-base-content/50">
          Pick a rule parameter to insert (each rule once)
        </p>
        <button
          v-for="(opt, idx) in filteredPlaceholderOptions"
          :key="opt.insertText"
          type="button"
          class="w-full px-3 py-2 text-left text-sm hover:bg-base-200 focus:bg-base-200 focus:outline-none"
          :class="{ 'bg-primary/10': idx === safePlaceholderIndex }"
          @click="insertPlaceholder(opt)"
        >
          <span class="font-medium">{{ opt.parameterName }}</span>
          <span class="ml-1 text-base-content/50">({{ opt.conditionName }})</span>
        </button>
        <p v-if="!filteredPlaceholderOptions.length" class="px-3 py-2 text-xs text-base-content/50 italic">
          No parameters available or all rules already used.
        </p>
      </div>
    </div>
    <!-- rule panel -->
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
      <SemanticRuleList
        title="Used in text"
        empty-message="No rules used yet."
        :conditions="usedConditions"
        :is-param-used-in-text="isParamUsedInText"
        :is-param-required-and-unused="isParamRequiredAndUnused"
        :highlight-rule-title="true"
        @highlight-rule="(id) => setHighlight({ conditionId: id })"
        @highlight-param="(id, name) => setHighlight({ conditionId: id, parameterName: name })"
        @clear-highlight="clearHighlight"
        @insert-placeholder="onInsertPlaceholderFromPanel"
      />
      <SemanticRuleList
        title="Not used"
        empty-message="All rules used or none defined."
        :conditions="unusedConditions"
        @highlight-rule="(id) => setHighlight({ conditionId: id })"
        @highlight-param="(id, name) => setHighlight({ conditionId: id, parameterName: name })"
        @clear-highlight="clearHighlight"
        @insert-placeholder="onInsertPlaceholderFromPanel"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import type { SemanticCondition } from '@template-repository/models/contract-templace'
import {
  useClauseTextChips,
  conditionIdsInText,
  parseSegments,
  isPlaceholder,
} from '@template-repository/composables/useClauseTextChips'
import {
  usePlaceholderDropdownPosition,
  type PlaceholderDropdownMode,
} from '@template-repository/composables/usePlaceholderDropdownPosition'
import SemanticRuleList from '@template-repository/components/clauses-editor/SemanticRuleList.vue'

const PLACEHOLDER_DROPDOWN_MODE: PlaceholderDropdownMode = 'caret'

const props = defineProps<{
  /** Clause template string: plain text plus {{conditionId.parameterName}} placeholders. */
  modelValue: string
  semanticConditions: SemanticCondition[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const uiStore = useTemplateEditorUiStore()
const { clausePlaceholderHighlight } = storeToRefs(uiStore)

const editorRef = ref<HTMLDivElement | null>(null)
let valueFromEditor = false
const isMounted = ref(true)

const {
  getTemplateText,
  getCursorIndex,
  handlePaste,
  insertNewlineAtSelection,
  setCursorAfter,
  setCursorAt,
  syncFromTemplateText,
  applyHighlight,
  wrapSpaces,
} = useClauseTextChips(editorRef, clausePlaceholderHighlight, isMounted)

const {
  dropdownStyle: placeholderDropdownStyle,
  dropdownClass: placeholderDropdownClass,
  updatePosition: updatePlaceholderDropdownPosition,
  clearPosition: clearPlaceholderDropdownPosition,
} = usePlaceholderDropdownPosition(editorRef, PLACEHOLDER_DROPDOWN_MODE)

const placeholderInsertStart = ref(-1)
const placeholderInsertEnd = ref(0)
const placeholderFilter = ref('')
const selectedPlaceholderIndex = ref(0)
/** Last cursor index in the editor */
const lastCursorIndex = ref(0)

const usedConditionIds = computed(() => conditionIdsInText(props.modelValue))
const usedPlaceholderKeys = computed(() => {
  const segs = parseSegments(props.modelValue, props.semanticConditions)
  const set = new Set<string>()
  segs.filter((s) => isPlaceholder(s)).forEach((s) => set.add(`${s.conditionId}.${s.parameterName}`))
  return set
})
const usedConditions = computed(() => props.semanticConditions.filter((c) => usedConditionIds.value.has(c.conditionId)))
const unusedConditions = computed(() =>
  props.semanticConditions.filter((c) => !usedConditionIds.value.has(c.conditionId)),
)

/** Check if a condition parameter is used in the clause text (via a placeholder) */
function isParamUsedInText(conditionId: string, parameterName: string): boolean {
  const segments = parseSegments(props.modelValue, props.semanticConditions)
  return segments.some((s) => isPlaceholder(s) && s.conditionId === conditionId && s.parameterName === parameterName)
}

function isParamRequiredAndUnused(conditionId: string, parameterName: string): boolean {
  const cond = props.semanticConditions.find((c) => c.conditionId === conditionId)
  const param = cond?.parameters.find((p) => p.parameterName === parameterName)
  return !!(param?.isRequired && !isParamUsedInText(conditionId, parameterName))
}

function setHighlight(payload: { conditionId: string; parameterName?: string }) {
  uiStore.setClausePlaceholderHighlight(payload)
}

function clearHighlight() {
  uiStore.setClausePlaceholderHighlight(null)
}

/** Handles clicks from the rule panel and inserts the corresponding placeholder if not already used,
 *  or if it's required and currently unused.
 */
function onInsertPlaceholderFromPanel(conditionId: string, parameterName: string) {
  const paramNotUsedInText = !isParamUsedInText(conditionId, parameterName)
  if (paramNotUsedInText) {
    insertPlaceholderFromPanel(conditionId, parameterName)
  }
}

/** Inserts placeholder at lastCursorIndex. */
function insertPlaceholderFromPanel(conditionId: string, parameterName: string) {
  const insertText = `{{${conditionId}.${parameterName}}}`
  const current = props.modelValue ?? ''
  const len = current.length
  const insertPos = Math.max(0, Math.min(lastCursorIndex.value, len))
  const before = current.slice(0, insertPos)
  const after = current.slice(insertPos)
  const { value: newValue, insertLength } = wrapSpaces(before, insertText, after)
  const newCursorPos = insertPos + insertLength
  applyEditorChange(newValue, newCursorPos)
}

function onEditorBlur() {
  if (editorRef.value) lastCursorIndex.value = getCursorIndex()
}

const placeholderOptions = computed(() => {
  const usedKeys = usedPlaceholderKeys.value
  const list: { insertText: string; parameterName: string; conditionName: string }[] = []
  for (const c of props.semanticConditions) {
    for (const p of c.parameters) {
      if (usedKeys.has(`${c.conditionId}.${p.parameterName}`)) continue
      list.push({
        insertText: `{{${c.conditionId}.${p.parameterName}}}`,
        parameterName: p.parameterName,
        conditionName: c.conditionName,
      })
    }
  }
  return list
})

const filteredPlaceholderOptions = computed(() => {
  const q = placeholderFilter.value.trim().toLowerCase()
  if (!q) return placeholderOptions.value
  return placeholderOptions.value.filter(
    (opt) =>
      opt.parameterName.toLowerCase().includes(q) ||
      opt.conditionName.toLowerCase().includes(q) ||
      opt.insertText.toLowerCase().includes(q),
  )
})

const showPlaceholderSuggestions = computed(
  () => placeholderOptions.value.length > 0 && placeholderInsertStart.value >= 0,
)

const safePlaceholderIndex = computed(() => {
  const len = filteredPlaceholderOptions.value.length
  return Math.min(Math.max(0, selectedPlaceholderIndex.value), len > 0 ? len - 1 : 0)
})

function closePlaceholderSuggestions() {
  placeholderInsertStart.value = -1
  placeholderFilter.value = ''
  selectedPlaceholderIndex.value = 0
  clearPlaceholderDropdownPosition()
}

function onEditorClick(e: MouseEvent) {
  const target = e.target as Node
  const el = editorRef.value
  if (!el?.contains(target)) return
  let node: Node | null = target
  while (node && node !== el) {
    if (node.nodeType === Node.ELEMENT_NODE) {
      const span = node as HTMLElement
      // update cursor position when clicking on an existing placeholder
      if (span.dataset.conditionId != null && span.dataset.parameterName != null) {
        e.preventDefault()
        void nextTick(() => {
          el.focus()
          setCursorAfter(node!)
        })
        return
      }
    }
    node = node.parentNode
  }
}

function onEditorPaste(e: ClipboardEvent) {
  const result = handlePaste(e)
  applyEditorChange(result.newValue, result.newCursorPos)
}

function onEditorInput() {
  const el = editorRef.value
  if (!el) return
  const value = getTemplateText()
  const pos = getCursorIndex()
  lastCursorIndex.value = pos
  valueFromEditor = true
  emit('update:modelValue', value)

  const textBefore = value.slice(0, pos)
  const lastOpen = textBefore.lastIndexOf('{{')
  if (lastOpen === -1) {
    closePlaceholderSuggestions()
    return
  }
  const afterOpen = textBefore.slice(lastOpen + 2)
  if (afterOpen.includes('}}')) {
    closePlaceholderSuggestions()
    return
  }
  placeholderInsertStart.value = lastOpen
  placeholderInsertEnd.value = pos
  placeholderFilter.value = afterOpen
  selectedPlaceholderIndex.value = 0
}

function onEditorKeydown(e: KeyboardEvent) {
  const suggestionsVisible = showPlaceholderSuggestions.value
  if (e.key === 'Enter') {
    const list = filteredPlaceholderOptions.value
    // If placeholder suggestions are visible and Enter (without Shift) is pressed, confirm the current suggestion.
    if (suggestionsVisible && !e.shiftKey && list.length > 0) {
      e.preventDefault()
      const idx = Math.min(selectedPlaceholderIndex.value, list.length - 1)
      const opt = list[idx]
      if (opt) insertPlaceholder(opt)
      return
    }

    // In all other cases, treat Enter as inserting a logical newline.
    e.preventDefault()
    const { newValue, newCursorPos } = insertNewlineAtSelection()
    applyEditorChange(newValue, newCursorPos)
    return
  }

  if (!suggestionsVisible) return

  const list = filteredPlaceholderOptions.value
  if (e.key === 'Escape') {
    closePlaceholderSuggestions()
    e.preventDefault()
    return
  }
  if (e.key === 'ArrowDown' && list.length > 0) {
    e.preventDefault()
    selectedPlaceholderIndex.value = (selectedPlaceholderIndex.value + 1) % list.length
    return
  }
  if (e.key === 'ArrowUp' && list.length > 0) {
    e.preventDefault()
    selectedPlaceholderIndex.value = (selectedPlaceholderIndex.value - 1 + list.length) % list.length
    return
  }
}

// Insert placeholder by clicking in the suggestion panel
function insertPlaceholder(opt: { insertText: string }) {
  const el = editorRef.value
  const start = placeholderInsertStart.value
  const end = placeholderInsertEnd.value
  if (start < 0 || !el) {
    closePlaceholderSuggestions()
    return
  }
  const value = props.modelValue
  const before = value.slice(0, start)
  const after = value.slice(end)
  const { value: newValue, insertLength } = wrapSpaces(before, opt.insertText, after)
  closePlaceholderSuggestions()
  const newPos = start + insertLength
  applyEditorChange(newValue, newPos)
}

function applyEditorChange(newValue: string, newCursorPos: number) {
  valueFromEditor = false
  emit('update:modelValue', newValue)
  lastCursorIndex.value = newCursorPos
  void nextTick(() => {
    if (!isMounted.value) return
    syncFromTemplateText(newValue, props.semanticConditions)
    void nextTick(() => {
      if (isMounted.value && editorRef.value) {
        editorRef.value.focus()
        setCursorAt(editorRef.value, newCursorPos)
      }
    })
  })
}

/** When modelValue changes from outside (e.g. parent), sync editor DOM.
 *  Skip if the change came from user input so we don't overwrite the editor.
 */
watch(
  () => props.modelValue,
  () => {
    if (valueFromEditor) {
      valueFromEditor = false
      return
    }
    void nextTick(() => {
      if (isMounted.value) syncFromTemplateText(props.modelValue, props.semanticConditions)
    })
  },
)

watch(
  clausePlaceholderHighlight,
  () => {
    if (isMounted.value) applyHighlight()
    void nextTick(() => {
      if (isMounted.value) applyHighlight()
    })
  },
  { deep: true },
)

watch(showPlaceholderSuggestions, (visible) => {
  if (visible) void nextTick(() => updatePlaceholderDropdownPosition())
})

onMounted(() => {
  void nextTick(() => syncFromTemplateText(props.modelValue, props.semanticConditions))
  if (PLACEHOLDER_DROPDOWN_MODE === 'caret' && editorRef.value) {
    editorRef.value.addEventListener('scroll', updatePlaceholderDropdownPosition)
  }
})

onBeforeUnmount(() => {
  isMounted.value = false
  if (PLACEHOLDER_DROPDOWN_MODE === 'caret' && editorRef.value) {
    editorRef.value.removeEventListener('scroll', updatePlaceholderDropdownPosition)
  }
})
</script>

<style scoped>
.clause-editor:empty::before {
  content: attr(data-placeholder);
  color: oklch(var(--bc) / 0.4);
}

.clause-editor :deep(.clause-chip-highlight) {
  border-style: solid;
  border-color: oklch(var(--p));
  box-shadow: none;
}
</style>
