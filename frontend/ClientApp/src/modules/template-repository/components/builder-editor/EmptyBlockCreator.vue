<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const { layout } = storeToRefs(useDcsDraftStore())
const uiStore = useTemplateEditorUiStore()

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))

function openAddBlockAtRoot() {
  const root = rootBlock.value
  if (root) uiStore.openAddBlockModal(root['@id'], 0)
}
</script>

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
