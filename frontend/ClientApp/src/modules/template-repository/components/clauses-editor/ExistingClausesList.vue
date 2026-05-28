<template>
  <div class="space-y-2">
    <p v-if="!clauseBlocks.length" class="py-6 text-center text-xs text-base-content/40 italic">
      No clauses defined yet.
    </p>
    <div
      v-for="clause in clauseBlocks"
      :key="clause.blockId"
      class="group flex items-start gap-3 rounded-lg border border-base-300 bg-base-200/30 p-3 transition-all hover:shadow-sm"
    >
      <div class="min-w-0 flex-1">
        <div v-if="editingBlockId === clause.blockId">
          <ClauseEditorForm
            :mode="'edit'"
            :initial-title="clause.title ?? ''"
            :initial-text="clause.text ?? ''"
            :semantic-conditions="semanticConditions"
            @submit="(payload) => $emit('save', { blockId: clause.blockId, ...payload })"
            @cancel="$emit('cancel-edit')"
          />
        </div>
        <div v-else>
          <div class="text-sm font-semibold text-base-content">
            {{ clause.title ?? '' }}
            <span v-if="outlineBlockIds.has(clause.blockId)" class="ml-1 font-normal text-base-content/60">
              (used in builder)
            </span>
          </div>
          <p class="mt-1 text-xs leading-relaxed whitespace-pre-wrap text-base-content/70">
            <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
          </p>
        </div>
      </div>
      <!-- Actions -->
      <div
        v-if="editable && editingBlockId !== clause.blockId"
        class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100"
      >
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          aria-label="Edit clause"
          @click="$emit('edit', clause.blockId)"
        >
          <IconEdit class="h-4 w-4" />
        </button>
        <button
          type="button"
          class="btn text-error btn-ghost btn-xs"
          aria-label="Delete clause"
          @click="$emit('delete', clause.blockId)"
        >
          <IconRemove class="h-4 w-4" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ClauseBlock, SemanticCondition } from '@template-repository/models/contract-templace'
import {
  parseSegments,
  getPlaceholderLabelFromConditions,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import IconEdit from '@/core/components/icons/IconEdit.vue'
import IconRemove from '@/core/components/icons/IconRemove.vue'

const props = withDefaults(
  defineProps<{
    clauseBlocks: ClauseBlock[]
    semanticConditions: SemanticCondition[]
    blockIdsInOutline: Set<string>
    editingBlockId: string | null
    editable?: boolean
  }>(),
  { editable: true },
)

const outlineBlockIds = computed(() => props.blockIdsInOutline)

defineEmits<{
  delete: [blockId: string]
  edit: [blockId: string]
  save: [payload: { blockId: string; title: string; text: string }]
  'cancel-edit': []
}>()

function getSegments(clause: ClauseBlock): Segment[] {
  return parseSegments(clause.text ?? '', props.semanticConditions)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, props.semanticConditions)
}
</script>
