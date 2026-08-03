<script setup lang="ts">
import { useContractTemplateEventType } from '@/composables/useContractTemplateEventType'
import { toProperCase } from '@/utils/string'
import type { ContractTemplateAuditResponse } from '@/models/responses/template-response'

defineProps<{
  audits: ContractTemplateAuditResponse
}>()

const eventType = useContractTemplateEventType()

type TemplateAuditItem = ContractTemplateAuditResponse[number]

const isPolicyFinding = (audit: TemplateAuditItem) => String(audit.event_type) === 'TEMPLATE_POLICY_AUDIT_FINDING'

const policyField = (audit: TemplateAuditItem, key: string) => {
  const data = audit.event_data as unknown
  if (typeof data !== 'object' || data === null || Array.isArray(data)) return ''
  const value = (data as Record<string, unknown>)[key]
  return typeof value === 'string' || typeof value === 'number' ? String(value) : ''
}

const policyBadgeClass = (audit: TemplateAuditItem) => {
  const severity = policyField(audit, 'severity').toLowerCase()
  if (severity === 'error') return 'badge-error'
  if (severity === 'warning') return 'badge-warning'
  // 'satisfied' is a verdict this audit reached; 'deferred' is the absence of
  // one (ADR-33), so it does not share the passing badge.
  if (severity === 'satisfied') return 'badge-success'
  return 'badge-ghost'
}
</script>

<template>
  <ul class="list">
    <li v-for="audit in audits" :key="audit.id" class="list-row grid-cols-1">
      <div class="flex justify-between">
        <div>{{ new Date(audit.created_at).toLocaleString() }}</div>
        <div
          v-if="isPolicyFinding(audit)"
          class="badge badge-soft badge-sm font-medium"
          :class="policyBadgeClass(audit)"
        >
          {{ policyField(audit, 'severity') || 'finding' }}
        </div>
        <div v-else class="badge badge-soft badge-sm badge-secondary">{{ toProperCase(audit.event_type) }}</div>
        <div class="text-xs">{{ toProperCase(audit.component) }}</div>
      </div>
      <div class="list-col-wrap">
        <div v-if="isPolicyFinding(audit)" class="space-y-1">
          <div v-if="policyField(audit, 'objectName')" class="text-xs font-medium opacity-70">
            {{ policyField(audit, 'objectName') }}
            <span v-if="policyField(audit, 'state')">· {{ policyField(audit, 'state') }}</span>
            <span v-if="policyField(audit, 'templateType')">· {{ policyField(audit, 'templateType') }}</span>
          </div>
          <div class="font-medium">{{ policyField(audit, 'title') || 'Policy finding' }}</div>
          <div class="text-sm opacity-85">{{ policyField(audit, 'message') }}</div>
          <div class="text-xs opacity-70">
            {{ policyField(audit, 'ruleId') }}
            <span v-if="policyField(audit, 'fieldIri')">· {{ policyField(audit, 'fieldIri') }}</span>
            <span v-if="policyField(audit, 'requirement')">· {{ policyField(audit, 'requirement') }}</span>
          </div>
        </div>
        <div v-else-if="eventType.isCreateEvent(audit)">
          <div>Created by: {{ audit.event_data.created_by }}</div>
        </div>
        <div v-else-if="eventType.isCopyEvent(audit)">
          <div>Copied by: {{ audit.event_data.copied_by }}</div>
        </div>
        <div v-else-if="eventType.isSubmitEvent(audit)" class="flex justify-between">
          <div>Submitted by: {{ audit.event_data.submitted_by }}</div>
          <div>
            Transition:
            <span class="badge badge-soft badge-xs badge-secondary" aria-label="From state">
              {{ toProperCase(audit.event_data.previous_state) }}
            </span>
            <span aria-hidden="true">→</span>
            <span class="badge badge-soft badge-xs badge-secondary" aria-label="To state">
              {{ toProperCase(audit.event_data.new_state) }}
            </span>
          </div>
        </div>
        <div v-else-if="eventType.isApproveEvent(audit)">
          <div>Approved by: {{ audit.event_data.approved_by }}</div>
        </div>
        <div v-else-if="eventType.isRejectEvent(audit)" class="flex justify-between">
          <div>Rejected by: {{ audit.event_data.rejected_by }}</div>
          <div>Reason: {{ audit.event_data.reason }}</div>
        </div>
        <div v-else-if="eventType.isVerifyEvent(audit)">
          <div>Verified by: {{ audit.event_data.verified_by }}</div>
        </div>
        <div v-else-if="eventType.isUpdateEvent(audit)">
          <div>Updated by: {{ audit.event_data.updated_by }}</div>
        </div>
        <div v-else-if="eventType.isSearchEvent(audit)">
          <div>Retrieved by: {{ audit.event_data.retrieved_by }}</div>
        </div>
        <div v-else-if="eventType.isRetrieveAllEvent(audit)">
          <div>Retrieved by: {{ audit.event_data.retrieved_by }}</div>
        </div>
        <div v-else-if="eventType.isRetrieveByIDEvent(audit)">
          <div>Retrieved by: {{ audit.event_data.retrieved_by }}</div>
        </div>
        <div v-else-if="eventType.isArchiveEvent(audit)">
          <div>Archived by: {{ audit.event_data.archived_by }}</div>
        </div>
        <div v-else-if="eventType.isRegisterEvent(audit)">
          <div>Registered by: {{ audit.event_data.registered_by }}</div>
        </div>
        <div v-else-if="eventType.isPublishEvent(audit)">
          <div>Published by: {{ audit.event_data.published_by }}</div>
        </div>
        <div v-else-if="eventType.isAuditEvent(audit)">
          <div>Audited by: {{ audit.event_data.audited_by }}</div>
        </div>
        <div v-else>{{ audit.event_data }}</div>
      </div>
    </li>
  </ul>
</template>
