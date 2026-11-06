<script setup lang="ts">
import { ChevronDownIcon, ChevronRightIcon } from '@heroicons/vue/20/solid'
import { computed, ref, watch } from 'vue'
import { useContractEventType } from '@/composables/useContractEventType'
import {
  contractAuditEventActor,
  contractAuditEventDisplayText,
  contractAuditEventOccurredAt,
  contractAuditEventTransition,
} from '@/utils/contract-audit-event-display'
import { resolveAnnotationContentLabel } from '@/utils/review-block-annotations'
import { toProperCase } from '@/utils/string'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { ContractAuditResponse, ContractAuditResponseItem } from '@/models/responses/contract-response'

const props = defineProps<{
  audits: ContractAuditResponse
  /** Optional: resolve annotation targets to the same labels as Contract Content. */
  blocks?: DcsBlock[]
  layout?: DcsLayoutNode[]
}>()

const eventType = useContractEventType()
const expandedIds = ref<Set<number>>(new Set())

interface AuditAnnotationRow {
  blockId: string
  comment: string
  label: string
}

interface AuditRow {
  id: number
  title: string
  occurredAt: string
  actor?: string
  roles?: string
  eventType: string
  component: string
  transition?: string
  reason?: string
  comments?: string
  commentsLabel?: string
  annotations?: AuditAnnotationRow[]
  supersededIntro?: string
  superseded?: { key: string; text: string }[]
  eventDataJson: string
}

function formatOccurredAt(audit: ContractAuditResponseItem): string {
  const raw = contractAuditEventOccurredAt(audit.event_data, audit.created_at)
  if (!raw) return '—'
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
}

function formatEventDataJson(eventData: unknown): string {
  try {
    return JSON.stringify(eventData ?? null, null, 2)
  } catch {
    return String(eventData)
  }
}

function joinComments(comments: unknown): string | undefined {
  if (!Array.isArray(comments)) return undefined
  const parts = comments.map((c) => String(c ?? '').trim()).filter(Boolean)
  return parts.length ? parts.join('\n') : undefined
}

function parseAnnotations(raw: unknown): AuditAnnotationRow[] | undefined {
  if (!Array.isArray(raw)) return undefined
  const out: AuditAnnotationRow[] = []

  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const row = item as Record<string, unknown>
    const blockId = typeof row.block_id === 'string' ? row.block_id.trim() : ''
    const comment = typeof row.comment === 'string' ? row.comment.trim() : ''
    if (!blockId || !comment) continue
    out.push({
      blockId,
      comment,
      label: resolveAnnotationContentLabel(blockId, props.blocks ?? [], props.layout),
    })
  }

  return out.length ? out : undefined
}

function eventDataAnnotations(eventData: unknown): unknown {
  if (!eventData || typeof eventData !== 'object') return undefined
  return (eventData as Record<string, unknown>).annotations
}

function toRow(audit: ContractAuditResponseItem): AuditRow {
  const row: AuditRow = {
    id: audit.id,
    title: contractAuditEventDisplayText(audit.event_type, audit.event_data),
    occurredAt: formatOccurredAt(audit),
    actor: contractAuditEventActor(audit.event_data),
    eventType: String(audit.event_type),
    component: String(audit.component),
    transition: contractAuditEventTransition(audit.event_data),
    eventDataJson: formatEventDataJson(audit.event_data),
  }

  // Role attribution (DCS-FR-CWE-25): the roles the actor held for this action.
  const data = audit.event_data as unknown as Record<string, unknown>
  if (Array.isArray(data.user_roles) && data.user_roles.length) {
    row.roles = data.user_roles.map(String).join(', ')
  }
  // Approver comments (DCS-FR-CWE-15).
  const decisionNotes = joinComments(data.decision_notes)
  if (decisionNotes) row.comments = decisionNotes

  if (eventType.isSubmitEvent(audit)) {
    const comments = joinComments(audit.event_data.comments)
    if (comments) {
      row.comments = comments
      // Review Reject uses action_flag REJECT (SUBMITTED → NEGOTIATION).
      const prev = String(audit.event_data.previous_state ?? '').toUpperCase()
      const next = String(audit.event_data.new_state ?? '').toUpperCase()
      const reviewReturn = audit.event_data.action_flag === 'REJECT' || (prev === 'SUBMITTED' && next === 'NEGOTIATION')
      row.commentsLabel = reviewReturn ? 'Findings' : 'Comments'
    }
  }
  // Annotations live on SubmitEvent event_data; show whenever present (Review + Negotiate Audit History).
  row.annotations = parseAnnotations(eventDataAnnotations(audit.event_data))
  if (eventType.isRejectEvent(audit) && audit.event_data.reason) {
    row.reason = audit.event_data.reason
  }
  if (eventType.isNegotiationChangeSupersededEvent(audit)) {
    row.supersededIntro = `Version ${audit.event_data.merged_contract_version} does not carry these accepted change requests:`
    row.superseded = audit.event_data.superseded.map((discard) => ({
      key: `${discard.negotiation_id}-${discard.superseded_by}`,
      text: `${discard.negotiation_id}: ${discard.fields.join(', ')} superseded by ${discard.superseded_by}`,
    }))
  }

  return row
}

const rows = computed(() => {
  // Re-resolve annotation labels when contract content (blocks/layout) loads.
  void props.blocks
  void props.layout
  return props.audits.map(toRow)
})

watch(
  () => props.audits,
  () => {
    expandedIds.value = new Set()
  },
)

function isExpanded(id: number): boolean {
  return expandedIds.value.has(id)
}

function toggle(id: number) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIds.value = next
}
</script>

<template>
  <ul class="divide-y divide-base-300">
    <li v-for="row in rows" :key="row.id" class="py-3 first:pt-0 last:pb-0">
      <div class="flex items-start gap-2">
        <button
          type="button"
          class="btn mt-0.5 shrink-0 px-1 btn-ghost btn-xs"
          :aria-expanded="isExpanded(row.id)"
          :aria-label="isExpanded(row.id) ? 'Hide activity details' : 'Show activity details'"
          @click="toggle(row.id)"
        >
          <ChevronDownIcon v-if="isExpanded(row.id)" class="size-4" />
          <ChevronRightIcon v-else class="size-4" />
        </button>
        <div class="min-w-0 flex-1">
          <div class="text-sm font-medium text-base-content">{{ row.title }}</div>
          <div class="mt-0.5 text-xs text-base-content/70">{{ row.occurredAt }}</div>
          <div v-if="row.actor" class="mt-1 font-mono text-xs break-all text-base-content/80">By: {{ row.actor }}</div>
          <div v-if="row.roles" class="mt-1 text-xs text-base-content/80" data-testid="audit-roles">
            Role: {{ row.roles }}
          </div>
          <div v-if="row.transition" class="mt-1 text-sm text-base-content/90" data-testid="audit-transition">
            Transition: {{ row.transition }}
          </div>
          <div
            v-if="row.reason"
            class="mt-1 text-sm break-words whitespace-pre-wrap text-base-content/90"
            data-testid="audit-reason"
          >
            Reason: {{ row.reason }}
          </div>
          <div
            v-if="row.comments"
            class="mt-1 text-sm break-words whitespace-pre-wrap text-base-content/90"
            data-testid="audit-comments"
          >
            {{ row.commentsLabel ?? 'Comments' }}: {{ row.comments }}
          </div>
          <div v-if="row.annotations?.length" class="mt-2 text-sm text-base-content/90" data-testid="audit-annotations">
            <div class="font-medium">Annotations</div>
            <ul class="mt-1.5 list-none space-y-2 p-0">
              <li
                v-for="(ann, i) in row.annotations"
                :key="`${ann.blockId}-${i}`"
                class="rounded-box border border-base-content/10 bg-base-200/40 px-3 py-2 break-words"
              >
                <div class="text-xs text-base-content/70" :title="ann.blockId">
                  <span class="font-medium text-base-content/50">Content:</span>
                  {{ ann.label }}
                </div>
                <div class="mt-1 text-sm whitespace-pre-wrap text-base-content">
                  <span class="font-medium text-base-content/50">Annotation:</span>
                  {{ ann.comment }}
                </div>
              </li>
            </ul>
          </div>

          <div
            v-if="isExpanded(row.id)"
            class="mt-2 space-y-2 border-l border-base-300 pl-3 text-xs text-base-content/70"
          >
            <div>Event: {{ toProperCase(row.eventType) }}</div>
            <div>Component: {{ toProperCase(row.component) }}</div>
            <div v-if="row.reason" class="break-words whitespace-pre-wrap">Reason: {{ row.reason }}</div>
            <div v-if="row.comments" class="break-words whitespace-pre-wrap">
              {{ row.commentsLabel ?? 'Comments' }}: {{ row.comments }}
            </div>
            <div v-if="row.supersededIntro">{{ row.supersededIntro }}</div>
            <ul v-if="row.superseded?.length" class="list-disc pl-4">
              <li v-for="item in row.superseded" :key="item.key">{{ item.text }}</li>
            </ul>
            <details class="collapse-arrow collapse rounded-box border border-base-content/10">
              <summary class="collapse-title text-xs font-medium">Event data (JSON)</summary>
              <div class="collapse-content">
                <pre class="text-xs wrap-break-word whitespace-pre-wrap">{{ row.eventDataJson }}</pre>
              </div>
            </details>
          </div>
        </div>
      </div>
    </li>
  </ul>
</template>
