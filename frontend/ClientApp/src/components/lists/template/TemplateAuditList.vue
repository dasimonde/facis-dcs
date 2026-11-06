<script setup lang="ts">
import { ChevronDownIcon, ChevronRightIcon } from '@heroicons/vue/20/solid'
import { computed, ref, watch } from 'vue'
import { useContractTemplateEventType } from '@/composables/useContractTemplateEventType'
import { toProperCase } from '@/utils/string'
import { templateAuditEventActor, templateAuditEventDisplayText } from '@/utils/template-audit-event-display'
import type { ContractTemplateAuditResponse } from '@/models/responses/template-response'

const props = defineProps<{
  audits: ContractTemplateAuditResponse
}>()

const eventType = useContractTemplateEventType()
const expandedIds = ref<Set<number>>(new Set())

type TemplateAuditItem = ContractTemplateAuditResponse[number]

interface AuditRow {
  id: number
  title: string
  occurredAt: string
  actor?: string
  eventType: string
  component: string
  transition?: string
  reason?: string
  comments?: string
  commentsLabel?: string
  policyMessage?: string
  policyMeta?: string
  severity?: string
  severityClass?: string
  eventDataJson: string
}

function joinComments(comments: unknown): string | undefined {
  if (!Array.isArray(comments)) return undefined
  const parts = comments.map((c) => String(c ?? '').trim()).filter(Boolean)
  return parts.length ? parts.join('\n') : undefined
}

function formatEventDataJson(eventData: unknown): string {
  try {
    return JSON.stringify(eventData ?? null, null, 2)
  } catch {
    return String(eventData)
  }
}

function isPolicyFinding(audit: TemplateAuditItem) {
  return String(audit.event_type) === 'TEMPLATE_POLICY_AUDIT_FINDING'
}

function policyField(audit: TemplateAuditItem, key: string): string {
  const data = audit.event_data as unknown
  if (typeof data !== 'object' || data === null || Array.isArray(data)) return ''
  const value = (data as Record<string, unknown>)[key]
  return typeof value === 'string' || typeof value === 'number' ? String(value) : ''
}

function policyBadgeClass(audit: TemplateAuditItem): string {
  const severity = policyField(audit, 'severity').toLowerCase()
  if (severity === 'error') return 'badge-error'
  if (severity === 'warning') return 'badge-warning'
  if (severity === 'satisfied') return 'badge-success'
  return 'badge-ghost'
}

function formatOccurredAt(audit: TemplateAuditItem): string {
  const date = new Date(audit.created_at)
  return Number.isNaN(date.getTime()) ? audit.created_at : date.toLocaleString()
}

function toRow(audit: TemplateAuditItem): AuditRow {
  const row: AuditRow = {
    id: audit.id,
    title: templateAuditEventDisplayText(String(audit.event_type), audit.event_data),
    occurredAt: formatOccurredAt(audit),
    actor: templateAuditEventActor(audit.event_data),
    eventType: String(audit.event_type),
    component: String(audit.component),
    eventDataJson: formatEventDataJson(audit.event_data),
  }

  if (isPolicyFinding(audit)) {
    row.severity = policyField(audit, 'severity') || 'finding'
    row.severityClass = policyBadgeClass(audit)
    row.policyMessage = policyField(audit, 'message')
    const meta = [
      policyField(audit, 'objectName'),
      policyField(audit, 'state'),
      policyField(audit, 'templateType'),
      policyField(audit, 'ruleId'),
      policyField(audit, 'fieldIri'),
      policyField(audit, 'requirement'),
    ].filter(Boolean)
    row.policyMeta = meta.join(' · ')
  }

  if (eventType.isSubmitEvent(audit)) {
    row.transition = `${toProperCase(audit.event_data.previous_state)} → ${toProperCase(audit.event_data.new_state)}`
    const comments = joinComments(audit.event_data.comments)
    if (comments) {
      row.comments = comments
      row.commentsLabel = 'Comments'
    }
  }
  if (eventType.isRejectEvent(audit) && audit.event_data.reason) {
    row.reason = audit.event_data.reason
  }

  return row
}

const rows = computed(() => props.audits.map(toRow))

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
          <div class="flex flex-wrap items-center gap-2">
            <div class="text-sm font-medium text-base-content">{{ row.title }}</div>
            <span v-if="row.severity" class="badge badge-soft badge-sm font-medium" :class="row.severityClass">
              {{ row.severity }}
            </span>
          </div>
          <div class="mt-0.5 text-xs text-base-content/70">{{ row.occurredAt }}</div>
          <div v-if="row.actor" class="mt-1 font-mono text-xs break-all text-base-content/80">By: {{ row.actor }}</div>
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

          <div
            v-if="isExpanded(row.id)"
            class="mt-2 space-y-2 border-l border-base-300 pl-3 text-xs text-base-content/70"
          >
            <div>Event: {{ toProperCase(row.eventType) }}</div>
            <div>Component: {{ toProperCase(row.component) }}</div>
            <div v-if="row.transition">Transition: {{ row.transition }}</div>
            <div v-if="row.reason" class="break-words whitespace-pre-wrap">Reason: {{ row.reason }}</div>
            <div v-if="row.comments" class="break-words whitespace-pre-wrap">
              {{ row.commentsLabel ?? 'Comments' }}: {{ row.comments }}
            </div>
            <div v-if="row.policyMessage" class="break-words whitespace-pre-wrap">{{ row.policyMessage }}</div>
            <div v-if="row.policyMeta">{{ row.policyMeta }}</div>
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
