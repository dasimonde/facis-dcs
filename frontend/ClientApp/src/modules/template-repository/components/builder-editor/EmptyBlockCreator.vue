<template>
  <div class="border-2 border-dashed border-base-300 rounded-2xl p-8 text-center bg-base-200/50">
    <p class="text-base-content/70 mb-4">No blocks yet. Add your first block.</p>
    <button type="button" class="btn btn-sm bg-base-content text-base-100 border-0 shadow-lg hover:opacity-90"
      @click="openAddBlockAtRoot" :disabled="!uiStore.isTemplateEditable">
      Add block
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const { documentOutline } = storeToRefs(useTemplateDraftStore())
const uiStore = useTemplateEditorUiStore()

const rootBlock = computed(() => documentOutline.value.find((b) => b.isRoot))

function openAddBlockAtRoot() {
  const root = rootBlock.value
  if (root) uiStore.openAddBlockModal(root.blockId, 0)
}
</script>
