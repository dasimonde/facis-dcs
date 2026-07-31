<script setup lang="ts">
import { storeToRefs } from 'pinia'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'

const { layout, blocks, semanticConditions } = storeToRefs(useDcsDraftStore())
</script>

<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body">
      <h3 class="mb-3 card-title text-sm">Preview</h3>

      <div
        class="w-full max-w-4xl overflow-hidden rounded-md border border-base-300 bg-base-100 shadow-sm"
        style="aspect-ratio: 210 / 297"
      >
        <div class="h-full w-full overflow-auto px-10 py-8">
          <TemplatePreview :layout="layout" :blocks="blocks" :semantic-conditions="semanticConditions" />
        </div>
      </div>
    </div>
  </div>
</template>
