<template>
  <div>
    <p class="text-sm text-base-content/70 mb-2">Approved sub-templates:</p>

    <div v-if="templates.length" class="flex flex-col gap-2 max-h-64 overflow-y-auto">
      <div v-for="t in templates" :key="`${t.did}-${t.version}-${t.document_number}`"
        class="border border-base-300 rounded-lg bg-base-100">
        <div class="flex items-stretch px-3 py-2 cursor-pointer hover:bg-base-200 transition-colors">
          <!-- Collapse toggle icon on the left -->
          <button type="button"
            class="flex items-center justify-center w-8 mr-3 text-base-content/60 hover:text-base-content hover:bg-base-200/70 rounded-md transition-colors cursor-pointer"
            @click.stop="togglePreview(t.did)">
            <svg class="w-3 h-3 transition-transform duration-200"
              :class="expandedTemplateId === t.did ? 'rotate-180' : ''" xmlns="http://www.w3.org/2000/svg" fill="none"
              viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          <!-- Vertical divider -->
          <div class="w-px bg-base-300 mr-3" aria-hidden="true" />

        <div class="flex-1 min-w-0" @click="$emit('select', t)">
            <p class="text-sm font-medium text-base-content truncate">
              {{ t.name }}
            </p>
            <p class="text-xs text-base-content/70 mt-0.5 line-clamp-2">
              {{ t.description }}
            </p>
          </div>
          <!-- Reference count -->
          <span v-if="referenceCountByDid !== undefined"
            class="text-xs text-base-content/60 badge badge-ghost badge-sm shrink-0 self-center ml-2">
            {{ usedInTemplateLabel(t.did) }}
          </span>
        </div>

        <!-- Preview panel -->
        <div v-if="expandedTemplateId === t.did" class="border-t border-base-200 bg-base-200/60 px-3 py-3">
          <p class="text-xs font-medium text-base-content/70 mb-1.5">Preview template</p>
          <div class="max-h-64 overflow-auto bg-base-100 rounded-md border border-base-300 px-3 py-2">
            <TemplatePreview v-if="t.template_data" :document-outline="t.template_data.documentOutline"
              :document-blocks="t.template_data.documentBlocks"
              :semantic-conditions="t.template_data.semanticConditions" />
            <p v-else class="text-xs text-base-content/60 italic">
              No template data available.
            </p>
          </div>
        </div>
      </div>
    </div>

    <p v-else class="text-xs text-base-content/60 italic">
      No approved sub-templates available.
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'

const props = defineProps<{
  templates: SubTemplateSnapshot[]
  referenceCountByDid?: Record<string, number>
}>()

const emit = defineEmits<{
  (e: 'select', template: SubTemplateSnapshot): void
}>()

const expandedTemplateId = ref<string | null>(null)

function referenceCount(did: string): number {
  if (props.referenceCountByDid == null) return 0
  return props.referenceCountByDid[did] ?? 0
}

function usedInTemplateLabel(did: string): string {
  const n = referenceCount(did)
  if (n === 0) return 'Not used'
  return n === 1 ? 'Used once' : `Used ${n} times`
}

function togglePreview(templateId: string) {
  expandedTemplateId.value = expandedTemplateId.value === templateId ? null : templateId
}
</script>
