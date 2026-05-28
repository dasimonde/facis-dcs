<template>
  <div>
    <p class="mb-2 text-sm text-base-content/70">{{ title }}</p>

    <div v-if="templates.length" class="flex max-h-64 flex-col gap-2 overflow-y-auto">
      <div
        v-for="t in templates"
        :key="`${t.did}-${t.version}-${t.document_number}`"
        class="rounded-lg border border-base-300 bg-base-100"
      >
        <div class="flex cursor-pointer items-stretch px-3 py-2 transition-colors hover:bg-base-200">
          <!-- Collapse toggle icon on the left -->
          <button
            type="button"
            class="mr-3 flex w-8 cursor-pointer items-center justify-center rounded-md text-base-content/60 transition-colors hover:bg-base-200/70 hover:text-base-content"
            @click.stop="togglePreview(t.did)"
          >
            <svg
              class="h-3 w-3 transition-transform duration-200"
              :class="expandedTemplateId === t.did ? 'rotate-180' : ''"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          <!-- Vertical divider -->
          <div class="mr-3 w-px bg-base-300" aria-hidden="true" />

          <div class="min-w-0 flex-1" @click="$emit('select', t)">
            <p class="truncate text-sm font-medium text-base-content">
              {{ t.name }}
            </p>
            <p class="mt-0.5 line-clamp-2 text-xs text-base-content/70">
              {{ t.description }}
            </p>
          </div>
          <!-- Reference count -->
          <span
            v-if="referenceCountByDid !== undefined"
            class="ml-2 badge shrink-0 self-center badge-ghost text-xs badge-sm text-base-content/60"
          >
            {{ usedInTemplateLabel(t.did) }}
          </span>
        </div>

        <!-- Preview panel -->
        <div v-if="expandedTemplateId === t.did" class="border-t border-base-200 bg-base-200/60 px-3 py-3">
          <p class="mb-1.5 text-xs font-medium text-base-content/70">Preview template</p>
          <div class="max-h-64 overflow-auto rounded-md border border-base-300 bg-base-100 px-3 py-2">
            <TemplatePreview
              v-if="t.template_data"
              :document-outline="t.template_data.documentOutline"
              :document-blocks="t.template_data.documentBlocks"
              :semantic-conditions="t.template_data.semanticConditions"
            />
            <p v-else class="text-xs text-base-content/60 italic">No template data available.</p>
          </div>
        </div>
      </div>
    </div>

    <p v-else class="text-xs text-base-content/60 italic">No approved sub-templates available.</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'

const props = withDefaults(
  defineProps<{
    templates: SubTemplateSnapshot[]
    referenceCountByDid?: Record<string, number>
    title?: string
  }>(),
  {
    title: 'Approved sub-templates:',
  },
)

defineEmits<(e: 'select', template: SubTemplateSnapshot) => void>()

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
