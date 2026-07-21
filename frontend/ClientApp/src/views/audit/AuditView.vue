<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { auditingService } from '@/services/auditing-service'
import { useAuthStore } from '@/stores/auth-store'
import { downloadBlob as saveBlob } from '@/utils/download-blob'
import type { AuditReportFormat, AuditScope } from '@/models/requests/auditing-request'
import type { AuditFinding } from '@/models/responses/auditing-response'

const auditFindingsByScope = ref<Partial<Record<AuditScope, AuditFinding[]>>>({})
const auditErrorsByScope = ref<Partial<Record<AuditScope, string>>>({})
const executedAuditScopes = ref<Partial<Record<AuditScope, boolean>>>({})
const selectedFindingId = ref<number | string | null>(null)
const auditLoadingScope = ref<AuditScope | null>(null)
const reportLoadingScope = ref<AuditScope | null>(null)
const reportLoadingFormat = ref<AuditReportFormat | null>(null)
const selectedScope = ref<AuditScope>('contracts')
const didFilter = ref('')
const justification = ref('')
const authStore = useAuthStore()
const isArchiveManagerOnly = computed(
  () => authStore.user?.roles.includes('ARCHIVE_MANAGER') && !authStore.user?.roles.includes('AUDITOR'),
)
type AuditResult = 'passed' | 'failed' | 'review'
type AuditTab = 'checks' | 'timeline'
type TableFilterKey = 'result' | 'category' | 'component' | 'did'

const emptyValueLabel = '-'
const activeAuditTab = ref<AuditTab>('checks')
const tableFilters = ref<Record<TableFilterKey, Record<string, boolean>>>({
  result: {},
  category: {},
  component: {},
  did: {},
})

const allScopeOptions: { value: AuditScope; label: string }[] = [
  { value: 'templates', label: 'Templates' },
  { value: 'contracts', label: 'Contracts' },
  { value: 'signatures', label: 'Signatures' },
  { value: 'archive', label: 'Archive' },
]
const scopeOptions = computed(() =>
  isArchiveManagerOnly.value ? allScopeOptions.filter((scope) => scope.value === 'archive') : allScopeOptions,
)
watch(
  isArchiveManagerOnly,
  (restricted) => {
    if (restricted) selectedScope.value = 'archive'
  },
  { immediate: true },
)

const findings = computed(() => auditFindingsByScope.value[selectedScope.value] ?? [])
const error = computed(() => auditErrorsByScope.value[selectedScope.value] ?? null)
const hasExecutedAudit = computed(() => executedAuditScopes.value[selectedScope.value] === true)
const auditLoading = computed(() => auditLoadingScope.value !== null)
const reportLoading = computed(() => reportLoadingScope.value !== null)
const selectedAuditLoading = computed(() => auditLoadingScope.value === selectedScope.value)
const selectedReportLoading = computed(() => reportLoadingScope.value === selectedScope.value)

const filteredFindings = computed(() => {
  return checkFindings.value.filter((finding) => {
    return (
      tableFilterEnabled('result', auditResultLabel(finding)) &&
      tableFilterEnabled('category', finding.category) &&
      tableFilterEnabled('component', finding.component) &&
      tableFilterEnabled('did', finding.did)
    )
  })
})
const selectedFinding = computed(() => {
  return findings.value.find((finding) => String(finding.id) === String(selectedFindingId.value)) ?? null
})
const selectedFindingKind = computed(() => (selectedFinding.value ? auditItemKind(selectedFinding.value) : null))
const selectedFindingEventData = computed(() => {
  if (!selectedFinding.value) {
    return null
  }
  if (isObjectRecord(selectedFinding.value.details)) {
    const eventData = selectedFinding.value.details.event_data ?? selectedFinding.value.details.eventData
    return isObjectRecord(eventData) ? eventData : null
  }
  return null
})
const selectedFindingDetailRows = computed(() => {
  const finding = selectedFinding.value
  const eventData = selectedFindingEventData.value
  if (!finding) {
    return []
  }
  if (selectedFindingKind.value === 'event') {
    return [
      { label: 'Event Type', value: stringDetail(rawEventType(finding)) },
      { label: 'Actor', value: stringDetail(actorFromEventData(eventData)) },
      { label: 'Timestamp', value: stringDetail(formatDateTime(finding.created_at)) },
      { label: 'Component', value: stringDetail(finding.component) },
      { label: 'DID', value: stringDetail(finding.did) },
    ].filter((row) => row.value !== emptyValueLabel)
  }
  return [
    { label: 'Checked', value: stringDetail(checkAssertion(finding)) },
    { label: 'Rule ID', value: stringDetail(eventData?.ruleId) },
    { label: 'Policy Set', value: stringDetail(eventData?.policySetId) },
    { label: 'Policy Version', value: stringDetail(eventData?.policyVersion) },
    { label: 'Requirement', value: detailValue(eventData?.requirement) },
    { label: 'Actual value', value: detailValue(eventData?.actualValue) },
    { label: 'Expected value', value: detailValue(eventData?.expectedValue) },
    { label: 'Expected values', value: detailValue(eventData?.expectedValues) },
    { label: 'Operator', value: detailValue(eventData?.operator) },
    { label: 'Field IRI', value: stringDetail(eventData?.fieldIri) },
    { label: 'Path', value: stringDetail(eventData?.path) },
    { label: 'Ontology Term', value: stringDetail(eventData?.ontologyTerm) },
    { label: 'Object Type', value: stringDetail(eventData?.objectType ?? finding.object_type) },
    { label: 'Contract Version', value: stringDetail(eventData?.contractVersion) },
    { label: 'Audited By', value: stringDetail(eventData?.auditedBy) },
    { label: 'Component', value: stringDetail(finding.component) },
    { label: 'DID', value: stringDetail(finding.did) },
  ].filter((row) => row.value !== emptyValueLabel)
})

const checkFindings = computed(() => findings.value.filter((finding) => auditItemKind(finding) === 'check'))
const timelineEvents = computed(() => {
  return [...findings.value]
    .filter((finding) => auditItemKind(finding) === 'event')
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
})
const failedCheckCount = computed(
  () => checkFindings.value.filter((finding) => auditResult(finding) === 'failed').length,
)
const passedCheckCount = computed(
  () => checkFindings.value.filter((finding) => auditResult(finding) === 'passed').length,
)
const reviewCheckCount = computed(
  () => checkFindings.value.filter((finding) => auditResult(finding) === 'review').length,
)
const auditIsEmpty = computed(() => hasExecutedAudit.value && findings.value.length === 0 && !error.value)
const auditHasPassed = computed(
  () =>
    hasExecutedAudit.value &&
    checkFindings.value.length > 0 &&
    failedCheckCount.value === 0 &&
    reviewCheckCount.value === 0 &&
    !error.value,
)
const tableFilterGroups: { key: TableFilterKey; label: string }[] = [
  { key: 'result', label: 'Status' },
  { key: 'category', label: 'Finding' },
  { key: 'component', label: 'Component' },
  { key: 'did', label: 'DID' },
]
const tableFilterOptions = computed<Record<TableFilterKey, string[]>>(() => ({
  result: uniqueTableValues(checkFindings.value.map((finding) => auditResultLabel(finding))),
  category: uniqueTableValues(checkFindings.value.map((finding) => finding.category)),
  component: uniqueTableValues(checkFindings.value.map((finding) => finding.component)),
  did: uniqueTableValues(checkFindings.value.map((finding) => finding.did)),
}))

watch(
  tableFilterOptions,
  (options) => {
    for (const group of tableFilterGroups) {
      const current = tableFilters.value[group.key]
      const next: Record<string, boolean> = {}
      for (const option of options[group.key]) {
        next[option] = current[option] ?? true
      }
      tableFilters.value[group.key] = next
    }
  },
  { immediate: true },
)

watch(selectedScope, () => {
  selectedFindingId.value = null
  activeAuditTab.value = checkFindings.value.length > 0 ? 'checks' : 'timeline'
})

const executeAudit = async () => {
  const scope = selectedScope.value
  auditLoadingScope.value = scope
  auditErrorsByScope.value = { ...auditErrorsByScope.value, [scope]: undefined }
  executedAuditScopes.value = { ...executedAuditScopes.value, [scope]: true }
  try {
    const scopeFindings = await auditingService.audit({
      scope,
      did: didFilter.value.trim() || undefined,
      justification: justification.value.trim(),
    })
    auditFindingsByScope.value = { ...auditFindingsByScope.value, [scope]: scopeFindings }
    selectedFindingId.value = null
    activeAuditTab.value = scopeFindings.some((finding) => auditItemKind(finding) === 'check') ? 'checks' : 'timeline'
  } catch (err) {
    console.error('Audit Error:', err)
    auditErrorsByScope.value = {
      ...auditErrorsByScope.value,
      [scope]: err instanceof Error ? err.message : 'Audit could not be executed.',
    }
  } finally {
    auditLoadingScope.value = null
  }
}

const generateReport = async (format: AuditReportFormat) => {
  const scope = selectedScope.value
  reportLoadingScope.value = scope
  reportLoadingFormat.value = format
  auditErrorsByScope.value = { ...auditErrorsByScope.value, [scope]: undefined }
  try {
    const artifact = await auditingService.report({
      scope,
      format,
      did: didFilter.value.trim() || undefined,
      justification: justification.value.trim(),
    })
    downloadBlob(artifact.bytes, artifact.contentType, artifact.filename)
  } catch (err) {
    console.error('Audit Report Error:', err)
    auditErrorsByScope.value = { ...auditErrorsByScope.value, [scope]: 'Audit report could not be generated.' }
  } finally {
    reportLoadingScope.value = null
    reportLoadingFormat.value = null
  }
}

const formatLabel = (value: string) => value.split('_').join(' ')

const selectedFindingRawDetails = computed(() => {
  if (!selectedFinding.value?.details) {
    return ''
  }
  return JSON.stringify(selectedFinding.value.details, null, 2)
})

function tableValue(value?: string): string {
  const trimmed = value?.trim()
  return trimmed ?? emptyValueLabel
}

function uniqueTableValues(values: (string | undefined)[]): string[] {
  return Array.from(new Set(values.map(tableValue))).sort((a, b) => a.localeCompare(b))
}

function tableFilterEnabled(key: TableFilterKey, value?: string): boolean {
  const normalized = tableValue(value)
  return tableFilters.value[key][normalized] ?? true
}

function checkedTableFilterCount(key: TableFilterKey): number {
  return tableFilterOptions.value[key].filter((option) => tableFilters.value[key][option]).length
}

function setAllTableFilters(key: TableFilterKey, enabled: boolean): void {
  for (const option of tableFilterOptions.value[key]) {
    tableFilters.value[key][option] = enabled
  }
}

function downloadBlob(content: BlobPart, contentType: string, filename: string): void {
  saveBlob(new Blob([content], { type: contentType }), filename)
}

function selectFinding(finding: AuditFinding): void {
  selectedFindingId.value = finding.id
}

function selectTab(tab: AuditTab): void {
  activeAuditTab.value = tab
  selectedFindingId.value = null
}

function stringDetail(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return emptyValueLabel
}

function detailValue(value: unknown): string {
  if (typeof value === 'string' && value.trim()) {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  if (Array.isArray(value)) {
    const values = value.map((item) => detailValue(item)).filter((item) => item !== emptyValueLabel)
    return values.length ? values.join(', ') : emptyValueLabel
  }
  if (isObjectRecord(value)) {
    return JSON.stringify(value)
  }
  return emptyValueLabel
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function findingBadgeClass(finding: AuditFinding): 'badge-success' | 'badge-error' | 'badge-warning' {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'badge-success'
  }
  if (result === 'failed') {
    return 'badge-error'
  }
  return 'badge-warning'
}

function auditItemKind(finding: AuditFinding): 'check' | 'event' {
  const eventType = rawEventType(finding)?.toUpperCase()
  const eventData = eventDataFromFinding(finding)
  if (
    eventType?.includes('POLICY_AUDIT_FINDING') ||
    eventType?.includes('COMPLIANCE_FINDING') ||
    eventType?.includes('AUDIT_CHECK')
  ) {
    return 'check'
  }
  if (eventData?.ruleId || eventData?.policySetId || eventData?.fieldIri || eventData?.severity) {
    return 'check'
  }
  return 'event'
}

function auditResult(finding: AuditFinding): AuditResult {
  const value = finding.status?.trim().toLowerCase()
  if (
    value === 'passed' ||
    value === 'pass' ||
    value === 'success' ||
    value === 'successful' ||
    value === 'ok' ||
    value === 'compliant' ||
    value === 'info'
  ) {
    return 'passed'
  }
  if (
    value === 'failed' ||
    value === 'fail' ||
    value === 'error' ||
    value === 'critical' ||
    value === 'blocking' ||
    value === 'violation' ||
    value === 'non_compliant'
  ) {
    return 'failed'
  }
  if (value === 'warning' || value === 'warn') {
    return 'review'
  }
  if (finding.category === 'violation') {
    return 'failed'
  }
  if (finding.category === 'inconsistency') {
    return 'review'
  }
  return 'review'
}

function auditResultLabel(finding: AuditFinding): 'Passed' | 'Failed' | 'Warning' | 'Needs review' {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'Passed'
  }
  if (result === 'failed') {
    return 'Failed'
  }
  if (isWarningSeverity(finding)) {
    return 'Warning'
  }
  return 'Needs review'
}

function auditResultSummary(finding: AuditFinding): string {
  const assertion = checkAssertion(finding)
  const result = auditResult(finding)
  if (result === 'passed') {
    return assertion ? `Passed: ${assertion}` : 'Check passed'
  }
  if (result === 'failed') {
    return assertion ? `Failed: ${assertion}` : 'Check failed'
  }
  if (isWarningSeverity(finding)) {
    return assertion ? `Warning: ${assertion}` : 'Warning'
  }
  return assertion ? `Review: ${assertion}` : 'Review required'
}

function checkAssertion(finding: AuditFinding): string {
  const eventData = eventDataFromFinding(finding)
  const message = stringDetail(eventData?.message)
  if (message !== emptyValueLabel) {
    return message
  }
  const descriptionLine = finding.description
    ?.split('\n')
    .map((line) => line.trim())
    .find(
      (line) =>
        line &&
        !line.startsWith('Object DID:') &&
        !line.startsWith('Rule:') &&
        !line.startsWith('Semantic path:') &&
        !line.startsWith('Requirement:') &&
        !line.startsWith('Actual value:') &&
        !line.startsWith('Expected value:') &&
        !line.startsWith('Expected values:') &&
        !line.startsWith('Operator:'),
    )
  return descriptionLine ?? ''
}

function severityLabel(finding: AuditFinding): string {
  return finding.status?.trim() ?? 'not set'
}

function isWarningSeverity(finding: AuditFinding): boolean {
  const severity = finding.status?.trim().toLowerCase()
  return severity === 'warning' || severity === 'warn'
}

function severityBadgeClass(finding: AuditFinding): 'badge-success' | 'badge-error' | 'badge-warning' | 'badge-ghost' {
  const severity = finding.status?.trim().toLowerCase()
  if (
    severity === 'error' ||
    severity === 'critical' ||
    severity === 'blocking' ||
    severity === 'failed' ||
    severity === 'violation'
  ) {
    return 'badge-error'
  }
  if (severity === 'warning' || severity === 'warn') {
    return 'badge-warning'
  }
  if (
    severity === 'passed' ||
    severity === 'pass' ||
    severity === 'success' ||
    severity === 'ok' ||
    severity === 'compliant' ||
    severity === 'info'
  ) {
    return 'badge-success'
  }
  return 'badge-ghost'
}

function eventDataFromFinding(finding: AuditFinding): Record<string, unknown> | null {
  if (!isObjectRecord(finding.details)) {
    return null
  }
  const eventData = finding.details.event_data ?? finding.details.eventData
  return isObjectRecord(eventData) ? eventData : null
}

function rawEventType(finding: AuditFinding): string | undefined {
  if (!isObjectRecord(finding.details)) {
    return undefined
  }
  const eventType = finding.details.event_type ?? finding.details.eventType
  return typeof eventType === 'string' ? eventType : undefined
}

function actorFromEventData(eventData: Record<string, unknown> | null): string | undefined {
  if (!eventData) {
    return undefined
  }
  const explicitActor = eventData.actor ?? eventData.user ?? eventData.username ?? eventData.auditedBy
  if (typeof explicitActor === 'string') {
    return explicitActor
  }
  for (const [key, value] of Object.entries(eventData)) {
    if (key.endsWith('_by') && typeof value === 'string') {
      return value
    }
  }
  return undefined
}

function formatDateTime(value?: string): string {
  if (!value) {
    return emptyValueLabel
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}
</script>

<template>
  <div class="mb-4 flex justify-between p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>

  <section class="space-y-4 px-4">
    <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
      <div class="stats stats-vertical border border-base-content/10 bg-base-200 sm:stats-horizontal">
        <div class="stat">
          <div class="stat-title flex items-center gap-2">
            <span class="size-2 rounded-full bg-error/50"></span>
            Failed Checks
          </div>
          <div class="stat-value text-2xl text-base-content">{{ failedCheckCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title flex items-center gap-2">
            <span class="size-2 rounded-full bg-success/50"></span>
            Passed Checks
          </div>
          <div class="stat-value text-2xl text-base-content">{{ passedCheckCount }}</div>
        </div>
        <div class="stat">
          <div class="stat-title flex items-center gap-2">
            <span class="size-2 rounded-full bg-warning/50"></span>
            Needs Review
          </div>
          <div class="stat-value text-2xl text-base-content">{{ reviewCheckCount }}</div>
        </div>
      </div>

      <div class="flex flex-col gap-3 sm:flex-row">
        <label class="form-control w-full sm:w-48">
          <span class="label-text mb-1">Scope</span>
          <select
            v-model="selectedScope"
            class="select-bordered select rounded-box"
            :disabled="auditLoading || reportLoading"
          >
            <option v-for="scope in scopeOptions" :key="scope.value" :value="scope.value">
              {{ scope.label }}
            </option>
          </select>
        </label>

        <label class="form-control w-full sm:w-64">
          <span class="label-text mb-1">DID (optional)</span>
          <input
            v-model="didFilter"
            class="input-bordered input rounded-box"
            :disabled="auditLoading || reportLoading"
          />
        </label>

        <label class="form-control w-full sm:w-72">
          <span class="label-text mb-1">Audit justification</span>
          <input
            v-model="justification"
            required
            class="input-bordered input rounded-box"
            :disabled="auditLoading || reportLoading"
          />
        </label>

        <button
          class="btn rounded-box btn-primary sm:self-end"
          :disabled="auditLoading || reportLoading || !justification.trim()"
          @click="executeAudit"
        >
          <span v-if="selectedAuditLoading" class="loading loading-sm loading-spinner"></span>
          <span v-else>Execute Audit</span>
        </button>

        <div class="flex flex-wrap gap-2 sm:self-end">
          <button
            class="btn rounded-box btn-outline"
            :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
            @click="generateReport('json')"
          >
            <span
              v-if="selectedReportLoading && reportLoadingFormat === 'json'"
              class="loading loading-sm loading-spinner"
            ></span>
            <span v-else>JSON</span>
          </button>
          <button
            class="btn rounded-box btn-outline"
            :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
            @click="generateReport('csv')"
          >
            <span
              v-if="selectedReportLoading && reportLoadingFormat === 'csv'"
              class="loading loading-sm loading-spinner"
            ></span>
            <span v-else>CSV</span>
          </button>
          <button
            class="btn rounded-box btn-outline"
            :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
            @click="generateReport('pdf')"
          >
            <span
              v-if="selectedReportLoading && reportLoadingFormat === 'pdf'"
              class="loading loading-sm loading-spinner"
            ></span>
            <span v-else>PDF</span>
          </button>
        </div>
      </div>
    </div>

    <div v-if="selectedAuditLoading" class="p-4">Executing audit...</div>
    <div v-else-if="error" class="alert rounded-box alert-error">{{ error }}</div>
    <div v-if="auditHasPassed" class="alert rounded-box alert-success">
      Audit passed. No failed checks or review findings were returned.
    </div>
    <div v-if="auditIsEmpty" class="alert rounded-box alert-info">
      Audit completed successfully. No matching entries were found.
    </div>

    <div v-if="!selectedAuditLoading && !error" class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div class="overflow-x-auto rounded-box border border-base-content/10">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-base-content/10 px-4 py-3">
          <div role="tablist" class="tabs-box tabs">
            <button
              type="button"
              role="tab"
              class="tab"
              :class="activeAuditTab === 'checks' ? 'tab-active' : ''"
              @click="selectTab('checks')"
            >
              Checks
              <span class="ml-2 badge badge-sm">{{ checkFindings.length }}</span>
            </button>
            <button
              type="button"
              role="tab"
              class="tab"
              :class="activeAuditTab === 'timeline' ? 'tab-active' : ''"
              @click="selectTab('timeline')"
            >
              Timeline
              <span class="ml-2 badge badge-sm">{{ timelineEvents.length }}</span>
            </button>
          </div>

          <div v-if="activeAuditTab === 'checks'" class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium opacity-70">Table filters</span>
            <details v-for="group in tableFilterGroups" :key="group.key" class="dropdown">
              <summary class="btn rounded-box btn-outline btn-sm">
                {{ group.label }}
                <span class="badge badge-sm">
                  {{ checkedTableFilterCount(group.key) }}/{{ tableFilterOptions[group.key].length }}
                </span>
              </summary>
              <div
                class="dropdown-content z-10 mt-2 w-72 rounded-box border border-base-content/10 bg-base-100 p-3 shadow"
              >
                <div class="mb-2 flex justify-between gap-2">
                  <button type="button" class="btn btn-ghost btn-xs" @click="setAllTableFilters(group.key, true)">
                    All
                  </button>
                  <button type="button" class="btn btn-ghost btn-xs" @click="setAllTableFilters(group.key, false)">
                    None
                  </button>
                </div>
                <div class="max-h-64 space-y-1 overflow-auto">
                  <label
                    v-for="option in tableFilterOptions[group.key]"
                    :key="option"
                    class="flex min-h-8 items-center gap-2 rounded px-2 hover:bg-base-200"
                  >
                    <input
                      v-model="tableFilters[group.key][option]"
                      type="checkbox"
                      class="checkbox checkbox-sm checkbox-primary"
                    />
                    <span class="text-sm break-all">{{ group.key === 'category' ? formatLabel(option) : option }}</span>
                  </label>
                </div>
              </div>
            </details>
          </div>
        </div>
        <table v-if="activeAuditTab === 'checks'" class="table table-zebra">
          <thead>
            <tr>
              <th>Status</th>
              <th>DID</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="finding in filteredFindings"
              :key="finding.id"
              class="cursor-pointer"
              :class="String(selectedFindingId) === String(finding.id) ? 'bg-primary/10' : ''"
              tabindex="0"
              @click="selectFinding(finding)"
              @keydown.enter.prevent="selectFinding(finding)"
              @keydown.space.prevent="selectFinding(finding)"
            >
              <td>
                <span class="badge" :class="findingBadgeClass(finding)">
                  {{ auditResultLabel(finding) }}
                </span>
                <div class="mt-1 text-xs opacity-70">{{ formatLabel(finding.category) }}</div>
              </td>
              <td>{{ finding.did ?? '-' }}</td>
              <td class="max-w-xl min-w-72">
                <div class="font-medium">{{ checkAssertion(finding) || finding.title || 'Audit finding' }}</div>
                <div v-if="checkAssertion(finding) && finding.title" class="text-xs opacity-70">
                  {{ finding.title }}
                </div>
                <div class="text-xs opacity-70">{{ auditResultSummary(finding) }}</div>
              </td>
            </tr>
            <tr v-if="filteredFindings.length === 0">
              <td colspan="3" class="py-8 text-center opacity-70">
                {{
                  hasExecutedAudit ? 'No checks match the selected filters.' : 'Select a scope and execute an audit.'
                }}
              </td>
            </tr>
          </tbody>
        </table>

        <table v-else class="table table-zebra">
          <thead>
            <tr>
              <th>Time</th>
              <th>Event</th>
              <th>Actor</th>
              <th>DID</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="event in timelineEvents"
              :key="event.id"
              class="cursor-pointer"
              :class="String(selectedFindingId) === String(event.id) ? 'bg-primary/10' : ''"
              tabindex="0"
              @click="selectFinding(event)"
              @keydown.enter.prevent="selectFinding(event)"
              @keydown.space.prevent="selectFinding(event)"
            >
              <td class="whitespace-nowrap">{{ formatDateTime(event.created_at) }}</td>
              <td class="max-w-xl min-w-72">
                <div class="font-medium">{{ event.title ?? formatLabel(rawEventType(event) ?? 'Audit event') }}</div>
              </td>
              <td>{{ actorFromEventData(eventDataFromFinding(event)) ?? '-' }}</td>
              <td>{{ event.did ?? '-' }}</td>
            </tr>
            <tr v-if="timelineEvents.length === 0">
              <td colspan="4" class="py-8 text-center opacity-70">
                {{ hasExecutedAudit ? 'No timeline events were returned.' : 'Select a scope and execute an audit.' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <aside class="min-h-80 rounded-box border border-base-content/10 bg-base-100 xl:sticky xl:top-4 xl:self-start">
        <div class="flex items-center justify-between border-b border-base-content/10 px-4 py-3">
          <h3 class="font-bold">{{ selectedFindingKind === 'event' ? 'Event Details' : 'Check Details' }}</h3>
          <button v-if="selectedFinding" type="button" class="btn btn-ghost btn-xs" @click="selectedFindingId = null">
            Close
          </button>
        </div>
        <div v-if="!selectedFinding" class="p-4 text-sm opacity-70">
          Select a row to inspect the corresponding audit evidence.
        </div>
        <div v-else class="space-y-4 p-4">
          <div v-if="selectedFindingKind === 'check'">
            <div class="mb-2 badge" :class="findingBadgeClass(selectedFinding)">
              {{ auditResultLabel(selectedFinding) }}
            </div>
            <h4 class="leading-snug font-bold">
              {{ checkAssertion(selectedFinding) || selectedFinding.title || 'Audit finding' }}
            </h4>
            <div v-if="checkAssertion(selectedFinding) && selectedFinding.title" class="mt-1 text-sm opacity-70">
              {{ selectedFinding.title }}
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-2 text-xs opacity-80">
              <span>{{ formatLabel(selectedFinding.category) }}</span>
              <span class="badge badge-outline badge-sm" :class="severityBadgeClass(selectedFinding)">
                Severity: {{ severityLabel(selectedFinding) }}
              </span>
            </div>
          </div>

          <div v-else>
            <div class="mb-2 badge badge-outline">Timeline event</div>
            <h4 class="leading-snug font-bold">
              {{ selectedFinding.title ?? formatLabel(rawEventType(selectedFinding) ?? 'Audit event') }}
            </h4>
            <div class="mt-2 text-xs opacity-80">{{ formatDateTime(selectedFinding.created_at) }}</div>
          </div>

          <p class="text-sm break-words whitespace-pre-wrap opacity-80">{{ selectedFinding.description }}</p>

          <dl class="divide-y divide-base-content/10 text-sm">
            <div
              v-for="row in selectedFindingDetailRows"
              :key="row.label"
              class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 py-2"
            >
              <dt class="font-medium opacity-70">{{ row.label }}</dt>
              <dd class="break-words">{{ row.value }}</dd>
            </div>
          </dl>

          <details
            v-if="selectedFindingRawDetails"
            class="collapse-arrow collapse rounded-box border border-base-content/10"
          >
            <summary class="collapse-title text-sm font-medium">Raw Details</summary>
            <div class="collapse-content">
              <pre class="text-xs break-words whitespace-pre-wrap">{{ selectedFindingRawDetails }}</pre>
            </div>
          </details>
        </div>
      </aside>
    </div>
  </section>
</template>
