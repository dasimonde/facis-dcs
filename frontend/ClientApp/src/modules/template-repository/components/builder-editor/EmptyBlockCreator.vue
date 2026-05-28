<template>
  <div class="rounded-2xl border-2 border-dashed border-base-300 bg-base-200/50 p-8 text-center">
    <p class="mb-4 text-base-content/70">No blocks yet. Add your first block.</p>
    <button
      type="button"
      class="btn border-0 bg-base-content text-base-100 shadow-lg btn-sm hover:opacity-90"
      :disabled="!uiStore.isTemplateEditable"
      @click="openAddBlockAtRoot"
    >
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
