<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  annotationCount,
  filterAnnotatableBlocks,
  incompleteAnnotationCount,
  listAnnotatableBlocks,
  type ReviewAnnotationDraft,
} from '@/utils/review-block-annotations'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'

const props = defineProps<{
  blocks: DcsBlock[]
  layout?: DcsLayoutNode[]
  modelValue: ReviewAnnotationDraft
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: ReviewAnnotationDraft]
}>()

const searchQuery = ref('')

const annotatable = computed(() => listAnnotatableBlocks(props.blocks, props.layout))

const count = computed(() => annotationCount(props.modelValue))
const incomplete = computed(() => incompleteAnnotationCount(props.modelValue))

const entries = computed(() =>
  annotatable.value
    .filter((b) => Object.prototype.hasOwnProperty.call(props.modelValue, b.blockId))
    .map((b) => ({
      ...b,
      comment: props.modelValue[b.blockId] ?? '',
    })),
)

const availableToAdd = computed(() =>
  filterAnnotatableBlocks(
    annotatable.value.filter((b) => !(b.blockId in props.modelValue)),
    searchQuery.value,
  ),
)

function setComment(blockId: string, comment: string) {
  const next = { ...props.modelValue }
  if (!comment.trim()) delete next[blockId]
  else next[blockId] = comment
  emit('update:modelValue', next)
}

function addBlock(blockId: string) {
  if (!blockId || props.disabled) return
  if (!(blockId in props.modelValue)) {
    emit('update:modelValue', { ...props.modelValue, [blockId]: '' })
  }
  searchQuery.value = ''
}

function remove(blockId: string) {
  if (props.disabled) return
  const next = { ...props.modelValue }
  delete next[blockId]
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="space-y-3" data-testid="review-block-annotations">
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h3 class="m-0 text-sm font-medium text-base-content">Annotations</h3>
      <span v-if="count" class="text-xs text-base-content/60">{{ count }} ready to send</span>
      <span v-else-if="incomplete" class="text-xs text-warning">{{ incomplete }} need text</span>
    </div>
    <p class="m-0 text-xs text-base-content/60">
      Pick contract text, write the annotation, then Approve or Reject. Empty rows are not sent.
    </p>

    <div v-if="!disabled" class="space-y-1">
      <span class="label-text text-xs">Search</span>
      <div class="overflow-hidden rounded-box border border-base-content/10">
        <input
          v-model="searchQuery"
          type="search"
          class="input input-sm w-full rounded-none border-0 border-b border-base-content/10 bg-transparent focus:ring-0 focus:outline-none"
          placeholder="Search…"
          autocomplete="off"
        />
        <ul v-if="availableToAdd.length" class="m-0 max-h-48 list-none space-y-0 overflow-y-auto p-0">
          <li v-for="b in availableToAdd" :key="b.blockId">
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-none px-3 py-1.5 text-left hover:bg-base-200"
              @click="addBlock(b.blockId)"
            >
              <span class="shrink-0 text-xs font-medium text-base-content/50">{{ b.type }}</span>
              <span class="min-w-0 flex-1 truncate text-sm text-base-content" :title="b.label">{{ b.label }}</span>
            </button>
          </li>
        </ul>
        <p v-else class="m-0 px-3 py-2 text-xs text-base-content/50">
          {{ searchQuery.trim() ? 'No matches.' : 'Nothing left to annotate.' }}
        </p>
      </div>
    </div>

    <ul v-if="entries.length" class="m-0 list-none space-y-3 p-0">
      <li v-for="entry in entries" :key="entry.blockId" class="rounded-box border border-base-content/10 p-3">
        <div class="mb-2 flex items-center justify-between gap-2">
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <span class="shrink-0 text-xs font-medium text-base-content/60">{{ entry.type }}</span>
            <span class="min-w-0 truncate text-sm text-base-content" :title="entry.preview || entry.label">
              {{ entry.label }}
            </span>
          </div>
          <button v-if="!disabled" type="button" class="btn shrink-0 btn-ghost btn-xs" @click="remove(entry.blockId)">
            Remove
          </button>
        </div>
        <textarea
          class="textarea-bordered textarea w-full textarea-sm"
          rows="2"
          :disabled="disabled"
          :value="entry.comment"
          placeholder="Annotation…"
          @input="setComment(entry.blockId, ($event.target as HTMLTextAreaElement).value)"
        />
      </li>
    </ul>
    <p v-else class="m-0 text-sm text-base-content/50">No annotations yet. Search and pick above.</p>
  </div>
</template>
