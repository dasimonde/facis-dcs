<template>
  <Teleport to="body">
    <div
      v-if="isPreviewDialogOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="builder-preview-title"
      @click.self="close"
    >
      <div class="mx-4 flex max-h-[90vh] w-full max-w-6xl flex-col rounded-2xl bg-base-100 shadow-xl" @click.stop>
        <!-- Header -->
        <div class="flex shrink-0 items-center justify-between border-b border-base-300 px-4 py-3">
          <h2 id="builder-preview-title" class="text-sm font-semibold">Preview</h2>
          <button type="button" class="btn btn-ghost btn-xs" aria-label="Close preview" @click="close">✕</button>
        </div>

        <!-- Content -->
        <div class="flex min-h-0 flex-1 justify-center overflow-auto p-4">
          <div class="flex w-full max-w-4xl justify-center">
            <!-- Display in A4 aspect ratio -->
            <div
              class="w-full max-w-225 overflow-hidden rounded-md border border-base-300 bg-base-100 shadow-sm"
              style="aspect-ratio: 210 / 297"
            >
              <div :class="previewContainerClasses">
                <TemplatePreview
                  :document-outline="documentOutline"
                  :document-blocks="documentBlocks"
                  :semantic-conditions="semanticConditions"
                  :sub-template-snapshots="subTemplateSnapshots"
                />
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
