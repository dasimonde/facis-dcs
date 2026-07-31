<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref, useId, watch } from 'vue'
import BlockToolbar from '@template-repository/components/builder-editor/toolbar/BlockToolbar.vue'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import { useBlockMovementPreview } from '@template-repository/composables/useBlockMovementPreview'
import {
  getPlaceholderLabelFromConditions,
  parseSegmentsFromContent,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import type { EnrichedBlockItem } from '@template-repository/models/enriched-block-item'

const props = defineProps<{
  item: EnrichedBlockItem
}>()

const emit = defineEmits<{
  select: []
  insertAbove: []
  insertBelow: []
  insertNest: []
  confirm: [payload: { title: string; text: string }]
  moveUp: []
  moveDown: []
  moveOutdent: []
  moveIndent: []
  delete: []
}>()

const uiStore = useTemplateEditorUiStore()
const draftStore = useDcsDraftStore()
const { selectedBlockId } = storeToRefs(uiStore)
const { semanticConditions } = storeToRefs(draftStore)
const { isSwapPreviewTarget } = useBlockMovementPreview()

const block = computed(() => props.item.block)

const clauseBlock = computed(() => (block.value?.['@type'] === 'dcs:Clause' ? block.value : undefined))

const clauseSegments = computed((): Segment[] => {
  const clause = clauseBlock.value
  if (!clause) return []
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  return parseSegmentsFromContent(content['@list'], semanticConditions.value)
})

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, semanticConditions.value)
}

const isSelected = computed(() => selectedBlockId.value === props.item.blockId)
const isSwapPreviewTargetForThis = computed(() => isSwapPreviewTarget(props.item.blockId))

const borderClass = computed(() => {
  if (isSwapPreviewTargetForThis.value) return 'border border-dashed border-primary'
  if (isSelected.value) return 'border border-primary'
  return 'border border-base-300'
})

const toolbarVisibilityClass = computed(() => {
  if (isSelected.value) return 'opacity-100'
  return 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100'
})

const savedTitle = computed(() => {
  const b = block.value
  if (b?.['@type'] === 'dcs:Section') return b['dcs:title'] ?? ''
  return ''
})
const savedText = computed(() => {
  const b = block.value
  if (b?.['@type'] === 'dcs:TextBlock') return b['dcs:text'] ?? ''
  if (b?.['@type'] === 'dcs:Section') return b['dcs:title'] ?? ''
  return ''
})

const localTitle = ref('')
const localText = ref('')

const localTitleId = useId()
const localTextId = useId()
const clauseLabelId = useId()

watch(
  () => [props.item.blockId, savedTitle.value, savedText.value] as const,
  ([, title, text]) => {
    localTitle.value = title
    localText.value = text
  },
  { immediate: true },
)

const isDirty = computed(() => {
  const b = block.value
  if (b?.['@type'] === 'dcs:Section') {
    return localTitle.value !== savedTitle.value
  }
  if (b?.['@type'] === 'dcs:TextBlock') {
    return localText.value !== savedText.value
  }
  return false
})

function onConfirm() {
  emit('confirm', { title: localTitle.value, text: localText.value })
}

function revertToSaved() {
  localTitle.value = savedTitle.value
  localText.value = savedText.value
}
</script>

<template>
  <div
    :class="[
      'group flex w-full items-start gap-2 rounded-lg border bg-base-100',
      'transition-[border-color,opacity] duration-200',
      borderClass,
    ]"
    :data-block-id="item.blockId"
    @click="emit('select')"
    @focusin="emit('select')"
  >
    <div class="min-w-0 flex-1 px-3 py-2">
      <!-- Section: title input -->
      <template v-if="block && block['@type'] === 'dcs:Section'">
        <label :for="localTitleId" class="text-[10px] font-bold uppercase opacity-60">Section</label>
        <input
          :id="localTitleId"
          v-model="localTitle"
          type="text"
          class="input input-sm mt-0.5 w-full input-ghost font-semibold"
          placeholder="Section title"
          :disabled="!uiStore.isTemplateEditable"
        />
      </template>
      <!-- Text: textarea -->
      <template v-else-if="block && block['@type'] === 'dcs:TextBlock'">
        <label :for="localTextId" class="text-[10px] font-bold uppercase opacity-60">Text</label>
        <textarea
          :id="localTextId"
          v-model="localText"
          :disabled="!uiStore.isTemplateEditable"
          class="textarea mt-0.5 min-h-10 w-full resize-y textarea-ghost text-sm textarea-sm"
          placeholder="Text content"
          rows="2"
        />
      </template>
      <!-- Clause: read-only -->
      <template v-else-if="block && block['@type'] === 'dcs:Clause'">
        <span :id="clauseLabelId" class="text-[10px] font-bold uppercase opacity-60">
          Clause
          <span class="mt-0.5 text-[10px] font-semibold text-base-content">
            ({{ clauseBlock?.['dcs:title'] ?? '' }})
          </span>
        </span>
        <p
          :aria-labelledby="clauseLabelId"
          class="mt-1 text-xs leading-relaxed whitespace-pre-wrap text-base-content/70"
        >
          <ClauseSegmentsPreview :segments="clauseSegments" :get-placeholder-label="getPlaceholderLabel" />
        </p>
      </template>
    </div>
    <div
      v-if="uiStore.isTemplateEditable"
      :class="['shrink-0 pt-2 pr-2 pb-2 transition-opacity', toolbarVisibilityClass]"
    >
      <BlockToolbar
        :item="item"
        :is-dirty="isDirty"
        @insert-above="emit('insertAbove')"
        @insert-below="emit('insertBelow')"
        @insert-nest="emit('insertNest')"
        @confirm="onConfirm"
        @cancel="revertToSaved"
        @move-up="emit('moveUp')"
        @move-down="emit('moveDown')"
        @move-outdent="emit('moveOutdent')"
        @move-indent="emit('moveIndent')"
        @delete="emit('delete')"
      />
    </div>
  </div>
</template>
