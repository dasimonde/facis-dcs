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
      <template v-if="block && isSectionBlock(block)">
        <label class="text-[10px] font-bold uppercase opacity-60">Section</label>
        <input
          v-model="localTitle"
          type="text"
          class="input input-sm mt-0.5 w-full input-ghost font-semibold"
          placeholder="Section title"
          :disabled="!uiStore.isTemplateEditable"
        />
      </template>
      <!-- Text: textarea -->
      <template v-else-if="block && isTextBlock(block)">
        <label class="text-[10px] font-bold uppercase opacity-60">Text</label>
        <textarea
          v-model="localText"
          :disabled="!uiStore.isTemplateEditable"
          class="textarea mt-0.5 min-h-10 w-full resize-y textarea-ghost text-sm textarea-sm"
          placeholder="Text content"
          rows="2"
        />
      </template>
      <!-- Clause: read-only -->
      <template v-else-if="block && isClauseBlock(block)">
        <label class="text-[10px] font-bold uppercase opacity-60">
          Clause
          <span class="mt-0.5 text-[10px] font-semibold text-base-content">({{ block.title ?? '' }})</span>
        </label>
        <p class="mt-1 text-xs leading-relaxed whitespace-pre-wrap text-base-content/70">
          <ClauseSegmentsPreview :segments="clauseSegments" :get-placeholder-label="getPlaceholderLabel" />
        </p>
      </template>
      <!-- Approved sub-template: read-only -->
      <template v-else-if="block && isApprovedTemplateBlock(block)">
        <label class="text-[10px] font-bold uppercase opacity-60">Sub template</label>
        <div class="mt-1 flex items-start gap-2">
          <!-- Collapse button -->
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-base-content/60 transition-colors hover:bg-base-200/70 hover:text-base-content"
            @click.stop="toggleApprovedPreview"
          >
            <svg
              class="h-3 w-3 transition-transform duration-200"
              :class="isApprovedPreviewOpen ? 'rotate-180' : ''"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium text-base-content">{{ approvedTemplateName }}</p>
            <p class="mt-0.5 line-clamp-2 text-xs text-base-content/70">{{ approvedTemplateDescription }}</p>
          </div>
        </div>
        <!-- Preview sub-template -->
        <div v-if="isApprovedPreviewOpen" class="mt-2 rounded-md bg-base-200/60 px-3 py-3">
          <p class="mb-1.5 text-xs font-medium text-base-content/70">Preview template</p>
          <div class="max-h-64 overflow-auto rounded-md border border-base-300 bg-base-100 px-3 py-2">
            <TemplatePreview
              v-if="approvedTemplate?.template_data"
              :document-outline="approvedTemplate.template_data.documentOutline"
              :document-blocks="approvedTemplate.template_data.documentBlocks"
              :semantic-conditions="approvedTemplate.template_data.semanticConditions"
              :sub-template-snapshots="subTemplateSnapshots"
            />
            <p v-else class="text-xs text-base-content/60 italic">No template data available.</p>
          </div>
        </div>
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
import {
  parseSegments,
  getPlaceholderLabelFromConditions,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
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
  { immediate: true },
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
