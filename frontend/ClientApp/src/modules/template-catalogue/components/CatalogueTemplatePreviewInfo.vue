<template>
  <div class="card bg-base-100 border border-base-300 shadow-sm">
    <div class="card-body">
      <h3 class="card-title text-sm mb-3">Preview</h3>

      <div
        class="bg-base-100 border border-base-300 shadow-sm rounded-md overflow-hidden w-full max-w-4xl"
        style="aspect-ratio: 210 / 297"
      >
        <div class="w-full h-full overflow-auto px-10 py-8">
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
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'

const { documentOutline, documentBlocks, semanticConditions, subTemplateSnapshots } = storeToRefs(useTemplateDraftStore())
</script>
