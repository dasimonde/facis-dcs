<template>
  <div class="space-y-6">
    <!-- Section 1: New clause -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <ClauseEditorForm mode="create" initial-title="" initial-text="" :semantic-conditions="semanticConditions"
        @submit="addClause" />
    </section>

    <!-- Section 2: Existing clauses -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="text-sm font-semibold text-base-content/80 mb-4">Existing clauses</h3>
      <ExistingClausesList :clause-blocks="clauseBlocks" :semantic-conditions="semanticConditions"
        :get-condition-name="getConditionName" @delete="deleteClause" :block-ids-in-outline="store.blockIdsInOutline"
        :editing-block-id="editingBlockId" @edit="startEditClause" @save="saveEditedClause" @cancel-edit="cancelEdit"
        :editable="uiStore.isTemplateEditable" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { isClauseBlock, type ClauseBlock } from '@template-repository/models/contract-templace'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { documentBlocks, semanticConditions } = storeToRefs(store)

const editingBlockId = ref<string | null>(null)

/** Extract conditionIds from clause text placeholders {{conditionId.parameterName}}. */
function conditionIdsFromText(text: string): string[] {
  const set = new Set<string>()
  const re = /\{\{([^}]+)\}\}/g
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    const inner = m[1] ?? ''
    const dot = inner.indexOf('.')
    const conditionId = dot >= 0 ? inner.slice(0, dot) : inner
    if (conditionId) set.add(conditionId)
  }
  return [...set]
}

const clauseBlocks = computed((): ClauseBlock[] =>
  documentBlocks.value.filter((b): b is ClauseBlock => isClauseBlock(b))
)

function getConditionName(conditionId: string): string {
  const c = semanticConditions.value.find((x) => x.conditionId === conditionId)
  return c?.conditionName ?? conditionId
}

function addClause(payload: { title: string; text: string }) {
  const text = payload.text.trim()
  if (!text) return
  store.addClause({
    title: payload.title.trim(),
    text,
    conditionIds: conditionIdsFromText(text),
  })
}

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; text: string }) {
  const text = payload.text.trim()
  const title = payload.title.trim()
  if (!text) return
  store.updateClause(payload.blockId, {
    title,
    text,
    conditionIds: conditionIdsFromText(text),
  })
  if (editingBlockId.value === payload.blockId) cancelEdit()
}

function deleteClause(blockId: string) {
  store.deleteClause(blockId)
  if (editingBlockId.value === blockId) cancelEdit()
}
</script>
