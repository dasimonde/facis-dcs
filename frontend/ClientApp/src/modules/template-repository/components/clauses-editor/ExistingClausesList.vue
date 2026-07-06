<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticCondition } from '@/modules/template-repository/models/contract-template'
import {
  parseSegmentsFromContent,
  getPlaceholderLabelFromConditions,
  type Segment,
} from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import IconEdit from '@/core/components/icons/IconEdit.vue'
import IconRemove from '@/core/components/icons/IconRemove.vue'
import type { DcsClause, DcsContentSegment } from '@/models/dcs-jsonld'

const props = withDefaults(
  defineProps<{
    clauseBlocks: DcsClause[]
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
  save: [payload: { blockId: string; title: string; content: DcsContentSegment[] }]
  place: [blockId: string]
  'cancel-edit': []
}>()

function clauseContent(clause: DcsClause): DcsContentSegment[] {
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  return content['@list']
}

function getSegments(clause: DcsClause): Segment[] {
  return parseSegmentsFromContent(clauseContent(clause), props.semanticConditions)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, props.semanticConditions)
}
</script>

<template>
  <div class="space-y-2">
    <p v-if="!clauseBlocks.length" class="py-6 text-center text-xs text-base-content/40 italic">
      Create clauses from Data Requirements to see them here.
    </p>
    <div
      v-for="clause in clauseBlocks"
      :key="clause['@id']"
      class="group flex items-start gap-3 rounded-lg border border-base-300 bg-base-200/30 p-3 transition-all hover:shadow-sm"
    >
      <div class="min-w-0 flex-1">
        <div v-if="editingBlockId === clause['@id']">
          <ClauseEditorForm
            :mode="'edit'"
            :initial-title="clause['dcs:title'] ?? ''"
            :initial-content="clauseContent(clause)"
            :semantic-conditions="semanticConditions"
            @submit="(payload) => $emit('save', { blockId: clause['@id'], ...payload })"
            @cancel="$emit('cancel-edit')"
          />
        </div>
        <div v-else>
          <div class="text-sm font-semibold text-base-content">
            {{ clause['dcs:title'] ?? '' }}
            <span
              class="ml-1 badge badge-sm"
              :class="outlineBlockIds.has(clause['@id']) ? 'badge-success' : 'badge-outline'"
            >
              {{ outlineBlockIds.has(clause['@id']) ? 'Placed' : 'Not placed' }}
            </span>
          </div>
          <p class="mt-1 text-xs leading-relaxed whitespace-pre-wrap text-base-content/70">
            <ClauseSegmentsPreview :segments="getSegments(clause)" :get-placeholder-label="getPlaceholderLabel" />
          </p>
        </div>
      </div>
      <!-- Actions -->
      <div
        v-if="editable && editingBlockId !== clause['@id']"
        class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100"
      >
        <button
          v-if="!outlineBlockIds.has(clause['@id'])"
          type="button"
          class="btn btn-xs btn-secondary"
          @click="$emit('place', clause['@id'])"
        >
          Place in document
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          aria-label="Edit clause"
          @click="$emit('edit', clause['@id'])"
        >
          <IconEdit class="h-4 w-4" />
        </button>
        <button
          type="button"
          class="btn text-error btn-ghost btn-xs"
          aria-label="Delete clause"
          @click="$emit('delete', clause['@id'])"
        >
          <IconRemove class="h-4 w-4" />
        </button>
      </div>
    </div>
  </div>
</template>
