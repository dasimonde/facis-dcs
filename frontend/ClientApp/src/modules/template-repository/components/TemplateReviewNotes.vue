<script setup lang="ts">
import type { TemplateReviewNote } from '@/models/contract-template/contract-template'

defineProps<{
  notes?: TemplateReviewNote[]
}>()

const kindLabels: Record<TemplateReviewNote['kind'], string> = {
  RETURNED_TO_DRAFT: 'Returned to draft',
  FORWARDED_TO_APPROVAL: 'Forwarded to approval',
  RETURNED_TO_REVIEW: 'Returned to review',
  REJECTED: 'Rejected',
  APPROVED: 'Approved',
}
</script>

<template>
  <!-- DCS-IR-TR-04, DCS-IR-TR-05: the comments and reasons of the review and approval decisions. -->
  <section
    v-if="notes?.length"
    class="rounded-box border border-base-content/10 bg-base-200/40 p-4 text-sm"
    data-testid="template-review-notes"
  >
    <h3 class="mb-2 font-medium">Review notes</h3>
    <ol class="space-y-3">
      <li v-for="(note, i) in notes" :key="i" data-testid="template-review-note">
        <p class="font-medium">{{ kindLabels[note.kind] ?? note.kind }}</p>
        <p v-for="(line, j) in note.text" :key="j" class="break-words whitespace-pre-line">{{ line }}</p>
        <p class="text-base-content/70">
          <span v-if="note.by" class="break-all">{{ note.by }},</span>
          {{ new Date(note.created_at).toLocaleString() }}
        </p>
      </li>
    </ol>
  </section>
</template>
