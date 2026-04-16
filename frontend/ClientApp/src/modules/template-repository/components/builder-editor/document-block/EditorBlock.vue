<template>
  <div :class="[
    'flex items-start gap-2 w-full rounded-lg border bg-base-100 group',
    'transition-[border-color,opacity] duration-200',
    borderClass
  ]" :data-block-id="item.blockId" @click="emit('select')" @focusin="emit('select')">
    <div class="flex-1 min-w-0 py-2 px-3">
      <!-- Section: title input -->
      <template v-if="block && isSectionBlock(block)">
        <label class="text-[10px] uppercase font-bold opacity-60">Section</label>
        <input v-model="localTitle" type="text" class="input input-sm input-ghost w-full font-semibold mt-0.5"
          placeholder="Section title" :disabled="!uiStore.isTemplateEditable" />
      </template>
      <!-- Text: textarea -->
      <template v-else-if="block && isTextBlock(block)">
        <label class="text-[10px] uppercase font-bold opacity-60">Text</label>
        <textarea v-model="localText" :disabled="!uiStore.isTemplateEditable"
          class="textarea textarea-ghost textarea-sm w-full mt-0.5 text-sm min-h-[2.5rem] resize-y"
          placeholder="Text content" rows="2" />
      </template>
      <!-- Clause: read-only -->
      <template v-else-if="block && isClauseBlock(block)">
        <label class="text-[10px] uppercase font-bold opacity-60">Clause <span
            class="text-[10px] font-semibold mt-0.5 text-base-content">({{ block.title ?? '' }})</span></label>
        <p class="text-xs text-base-content/70 mt-1 leading-relaxed whitespace-pre-wrap">
          <ClauseSegmentsPreview :segments="clauseSegments" :get-placeholder-label="getPlaceholderLabel" />
        </p>
      </template>
      <!-- Approved sub-template: read-only -->
      <template v-else-if="block && isApprovedTemplateBlock(block)">
        <label class="text-[10px] uppercase font-bold opacity-60">Sub template</label>
        <div class="mt-1 flex items-start gap-2">
          <!-- Collapse button -->
          <button type="button"
            class="flex items-center justify-center w-8 h-8 text-base-content/60 hover:text-base-content hover:bg-base-200/70 rounded-md transition-colors cursor-pointer flex-shrink-0"
            @click.stop="toggleApprovedPreview">
            <svg class="w-3 h-3 transition-transform duration-200" :class="isApprovedPreviewOpen ? 'rotate-180' : ''"
              xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"
              aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          <div class="flex-1 min-w-0">
            <p class="text-sm font-medium text-base-content truncate">{{ approvedTemplateName }}</p>
            <p class="text-xs text-base-content/70 mt-0.5 line-clamp-2">{{ approvedTemplateDescription }}</p>
          </div>
        </div>
        <!-- Preview sub-template -->
        <div v-if="isApprovedPreviewOpen" class="mt-2 bg-base-200/60 px-3 py-3 rounded-md">
          <p class="text-xs font-medium text-base-content/70 mb-1.5">Preview template</p>
          <div class="max-h-64 overflow-auto bg-base-100 rounded-md border border-base-300 px-3 py-2">
            <TemplatePreview v-if="approvedTemplate?.template_data"
              :document-outline="approvedTemplate.template_data.documentOutline"
              :document-blocks="approvedTemplate.template_data.documentBlocks"
              :semantic-conditions="approvedTemplate.template_data.semanticConditions"
              :sub-template-snapshots="subTemplateSnapshots" />
            <p v-else class="text-xs text-base-content/60 italic">
              No template data available.
            </p>
          </div>
        </div>
      </template>
    </div>
    <div v-if="uiStore.isTemplateEditable" :class="[
      'pt-2 pr-2 pb-2 flex-shrink-0 transition-opacity',
      toolbarVisibilityClass,
    ]">
      <BlockToolbar :item="item" :is-dirty="isDirty" @insert-above="emit('insertAbove')"
        @insert-below="emit('insertBelow')" @insert-nest="emit('insertNest')" @confirm="onConfirm"
        @cancel="revertToSaved" @move-up="emit('moveUp')" @move-down="emit('moveDown')"
        @move-outdent="emit('moveOutdent')" @move-indent="emit('moveIndent')" @delete="emit('delete')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { EnrichedBlockItem } from '@template-repository/models/enriched-block-item'
import {
  isSectionBlock,
  isTextBlock,
  isClauseBlock,
  isApprovedTemplateBlock,
} from '@template-repository/models/contract-templace'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { useBlockMovementPreview } from '@template-repository/composables/useBlockMovementPreview'
import BlockToolbar from '@template-repository/components/builder-editor/toolbar/BlockToolbar.vue'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { parseSegments, getPlaceholderLabelFromConditions, type Segment } from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import type { SubTemplateSnapshot } from '@/models/contract-template'

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
const draftStore = useTemplateDraftStore()
const { selectedBlockId } = storeToRefs(uiStore)
const { semanticConditions, subTemplateSnapshots } = storeToRefs(draftStore)
const { isSwapPreviewTarget } = useBlockMovementPreview()

const clauseSegments = computed(() => {
  const b = block.value
  if (!b || !isClauseBlock(b)) return []
  return parseSegments(b.text ?? '', semanticConditions.value)
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

const block = computed(() => props.item.block)

const approvedTemplate = computed<SubTemplateSnapshot | undefined>(() => {
  const b = block.value
  if (!b || !isApprovedTemplateBlock(b)) return undefined
  return subTemplateSnapshots.value.find((t) => t.did === b.templateId)
})

const approvedTemplateName = computed(() => approvedTemplate.value?.name ?? '')
const approvedTemplateDescription = computed(() => approvedTemplate.value?.description ?? '')
const isApprovedPreviewOpen = ref(false)

function toggleApprovedPreview() {
  isApprovedPreviewOpen.value = !isApprovedPreviewOpen.value
}

const savedTitle = computed(() => {
  const b = block.value
  if (b && isSectionBlock(b)) return b.title ?? b.text ?? ''
  return ''
})
const savedText = computed(() => {
  const b = block.value
  if (b && isTextBlock(b)) return b.text ?? ''
  if (b && isSectionBlock(b)) return b.text ?? ''
  return ''
})

const localTitle = ref('')
const localText = ref('')

watch(
  () => [props.item.blockId, savedTitle.value, savedText.value] as const,
  ([, title, text]) => {
    localTitle.value = title
    localText.value = text
  },
  { immediate: true }
)

const isDirty = computed(() => {
  const b = block.value
  if (b && isSectionBlock(b)) {
    return localTitle.value !== savedTitle.value || localText.value !== savedText.value
  }
  if (b && isTextBlock(b)) {
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
