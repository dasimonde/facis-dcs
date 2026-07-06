<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import type { DcsClause, DcsContentSegment } from '@/models/dcs-jsonld'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { blocks, layout, semanticConditions: mainSemanticConditions, subTemplateSnapshots } = storeToRefs(store)
const { pendingClauseDraft } = storeToRefs(uiStore)

const editingBlockId = ref<string | null>(null)
const newClauseTitle = ref('')
const newClauseText = ref('')

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))

const clauseBlocks = computed((): DcsClause[] => {
  const mainClauses = blocks.value.filter((b): b is DcsClause => b['@type'] === 'dcs:Clause')
  const subTemplateClauses = subTemplateSnapshots.value.flatMap((subTemplate) => {
    const subBlocks = subTemplate.template_data
    if (!subBlocks || typeof subBlocks !== 'object') return []
    const doc = subBlocks as import('@/models/dcs-jsonld').DcsDocumentData
    if (!doc['dcs:documentStructure']) return []
    return doc['dcs:documentStructure']['dcs:blocks']['@list'].filter(
      (b): b is DcsClause => b['@type'] === 'dcs:Clause',
    )
  })
  return [...mainClauses, ...subTemplateClauses]
})

const semanticConditions = computed(() => {
  const subTemplateConditions = subTemplateSnapshots.value.flatMap((subTemplate) =>
    getSemanticConditionsFromTemplateData(subTemplate.template_data),
  )
  return [...mainSemanticConditions.value, ...subTemplateConditions]
})

const newClauseSemanticConditions = computed(() => semanticConditions.value)
const draftTitle = computed(() => pendingClauseDraft.value?.title ?? newClauseTitle.value)
const draftText = computed(() => pendingClauseDraft.value?.text ?? newClauseText.value)

function addClause(payload: { title: string; content: DcsContentSegment[] }) {
  const content = payload.content
  if (!content.length) return
  store.addClause({
    title: payload.title.trim(),
    content,
  })
  newClauseTitle.value = ''
  newClauseText.value = ''
  uiStore.clearPendingClauseDraft()
}

function cancelPendingClauseDraft() {
  uiStore.clearPendingClauseDraft()
  newClauseTitle.value = ''
  newClauseText.value = ''
}

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; content: DcsContentSegment[] }) {
  const title = payload.title.trim()
  if (!payload.content.length) return
  store.updateClause(payload.blockId, {
    title,
    content: payload.content,
  })
  if (editingBlockId.value === payload.blockId) cancelEdit()
}

function deleteClause(blockId: string) {
  store.deleteClause(blockId)
  if (editingBlockId.value === blockId) cancelEdit()
}

function placeClause(blockId: string) {
  const root = rootBlock.value
  if (!root) return
  uiStore.startClausePlacement(blockId)
  uiStore.openAddBlockModal(root['@id'], root['dcs:children']['@list'].length)
}
</script>

<template>
  <div class="space-y-6">
    <!-- Section 1: New clause -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <ClauseEditorForm
        mode="create"
        :initial-title="draftTitle"
        :initial-text="draftText"
        :semantic-conditions="newClauseSemanticConditions"
        :source-requirement-name="pendingClauseDraft?.sourceConditionName"
        :show-cancel="!!pendingClauseDraft"
        @submit="addClause"
        @cancel="cancelPendingClauseDraft"
      />
    </section>

    <!-- Section 2: Existing clauses -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Existing clauses</h3>
      <ExistingClausesList
        :clause-blocks="clauseBlocks"
        :semantic-conditions="semanticConditions"
        :block-ids-in-outline="store.blockIdsInOutline"
        :editing-block-id="editingBlockId"
        :editable="uiStore.isTemplateEditable"
        @delete="deleteClause"
        @edit="startEditClause"
        @place="placeClause"
        @save="saveEditedClause"
        @cancel-edit="cancelEdit"
      />
    </section>
  </div>
</template>
