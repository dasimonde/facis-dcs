<template>
  <Teleport to="body">
    <div v-if="isPreviewDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog" aria-modal="true" aria-labelledby="builder-preview-title" @click.self="close">
      <div class="bg-base-100 rounded-2xl shadow-xl w-full max-w-6xl mx-4 flex flex-col max-h-[90vh]" @click.stop>
        <!-- Header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-base-300 flex-shrink-0">
          <h2 id="builder-preview-title" class="text-sm font-semibold"> Preview </h2>
          <button type="button" class="btn btn-ghost btn-xs" aria-label="Close preview" @click="close">
            ✕
          </button>
        </div>

        <!-- Content -->
        <div class="flex-1 min-h-0 overflow-auto p-4 flex justify-center">
          <div class="w-full max-w-4xl flex justify-center">
            <!-- Display in A4 aspect ratio -->
            <div class="bg-base-100 border border-base-300 shadow-sm rounded-md overflow-hidden w-full max-w-[900px]"
              style="aspect-ratio: 210 / 297">
              <div :class="previewContainerClasses">
                <TemplatePreview :document-outline="documentOutline" :document-blocks="documentBlocks"
                  :semantic-conditions="semanticConditions" :sub-template-snapshots="subTemplateSnapshots" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'

const uiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()

const { isPreviewDialogOpen } = storeToRefs(uiStore)
const { documentOutline, documentBlocks, semanticConditions, subTemplateSnapshots } = storeToRefs(draftStore)

// This container is block, not flex
const previewContainerClasses = 'w-full h-full overflow-auto px-10 py-8'

function close() {
  uiStore.togglePreviewDialog()
}
</script>
