<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import type { DcsClause, DcsContentSegment } from '@/models/dcs-jsonld'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { blocks, layout, semanticConditions } = storeToRefs(store)

const editingBlockId = ref<string | null>(null)

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))

const clauseBlocks = computed((): DcsClause[] =>
  blocks.value.filter((b): b is DcsClause => b['@type'] === 'dcs:Clause'),
)

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; content: DcsContentSegment[] }) {
  const title = payload.title.trim()
  if (!payload.content.length) return
  store.updateClause(payload.blockId, { title, content: payload.content })
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
</template>
