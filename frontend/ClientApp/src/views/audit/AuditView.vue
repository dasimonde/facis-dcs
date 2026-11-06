<script setup lang="ts">
import { watchDebounced } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { type ArchiveErasureStatus, archiveService } from '@/services/archive-service'
import { auditingService } from '@/services/auditing-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { downloadBlob as saveBlob } from '@/utils/download-blob'
import { claimReportedHttpError } from '@/utils/report-action-error'
import ActionHistoryPanel from '@/views/audit/ActionHistoryPanel.vue'
import AccessAttempts from './AccessAttempts.vue'
import CheckpointProofPanel from './CheckpointProofPanel.vue'
import type { AuditReportFormat, AuditScope } from '@/models/requests/auditing-request'
import type { AuditFinding } from '@/models/responses/auditing-response'

type AuditPageTab = 'compliance' | 'action-history'

const auditFindingsByScope = ref<Partial<Record<AuditScope, AuditFinding[]>>>({})
const auditErrorsByScope = ref<Partial<Record<AuditScope, string>>>({})
const successfulAuditQueries = ref<Partial<Record<AuditScope, string>>>({})
const selectedFindingId = ref<number | string | null>(null)
const auditLoadingScope = ref<AuditScope | null>(null)
const reportLoadingScope = ref<AuditScope | null>(null)
const reportLoadingFormat = ref<AuditReportFormat | null>(null)
const reportError = ref('')
const selectedScope = ref<AuditScope>('contracts')
const didFilter = ref('')
const activeTab = ref<AuditPageTab>('compliance')

// Deep-link drill-down (DCS-FR-CSA-21): the archive dashboard links here
// with ?scope=archive&did=<did> so a row lands on that contract's trail.
const route = useRoute()
if (
  typeof route.query.scope === 'string' &&
  ['templates', 'contracts', 'archive', 'signatures', 'PROCESS_AUDIT_AND_COMPLIANCE'].includes(route.query.scope)
) {
  selectedScope.value = route.query.scope as AuditScope
}
if (typeof route.query.did === 'string' && route.query.did) {
  didFilter.value = route.query.did
}

function setActiveTab(tab: AuditPageTab) {
  activeTab.value = tab
}
const justification = ref('')
const authStore = useAuthStore()

// PAC /compliance audit: Auditor + Archive Manager (POST /pac/audit).
// Action history (contract/template audit trail): Auditor + Compliance Officer.
const canSeeComplianceAudit = computed(
  () => !!authStore.user?.roles.some((role) => role === 'AUDITOR' || role === 'ARCHIVE_MANAGER'),
)
const canSeeActionHistory = computed(
  () => !!authStore.user?.roles.some((role) => role === 'AUDITOR' || role === 'COMPLIANCE_OFFICER'),
)
const showAuditTabList = computed(() => canSeeComplianceAudit.value && canSeeActionHistory.value)
// DCS-FR-PACM-05: the Compliance Officer exports the latest audit report of the
// contracts and the archive it may read; it does not run audits.
const canExportReportOnly = computed(
  () => !canSeeComplianceAudit.value && !!authStore.user?.roles.includes('COMPLIANCE_OFFICER'),
)

watch(
  [canSeeComplianceAudit, canSeeActionHistory],
  () => {
    if (activeTab.value === 'compliance' && !canSeeComplianceAudit.value && canSeeActionHistory.value) {
      activeTab.value = 'action-history'
    } else if (activeTab.value === 'action-history' && !canSeeActionHistory.value && canSeeComplianceAudit.value) {
      activeTab.value = 'compliance'
    } else if (!canSeeComplianceAudit.value && canSeeActionHistory.value) {
      activeTab.value = 'action-history'
    } else if (canSeeComplianceAudit.value && !canSeeActionHistory.value) {
      activeTab.value = 'compliance'
    }
  },
  { immediate: true },
)

const isArchiveManagerOnly = computed(
  () => authStore.user?.roles.includes('ARCHIVE_MANAGER') && !authStore.user?.roles.includes('AUDITOR'),
)
type AuditResult = 'passed' | 'failed' | 'review' | 'not_evaluated'
type TableFilterKey = 'result' | 'category' | 'component' | 'did'

const emptyValueLabel = '-'
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
  { value: 'PROCESS_AUDIT_AND_COMPLIANCE', label: 'Configuration and compliance' },
]
const scopeOptions = computed(() =>
  isArchiveManagerOnly.value ? allScopeOptions.filter((scope) => scope.value === 'archive') : allScopeOptions,
)
const reportOnlyScopeOptions = allScopeOptions.filter(
  (scope) => scope.value === 'contracts' || scope.value === 'archive',
)
watch(
  isArchiveManagerOnly,
  (restricted) => {
    if (restricted) selectedScope.value = 'archive'
  },
  { immediate: true },
)

const error = computed(() => auditErrorsByScope.value[selectedScope.value] ?? null)
const currentAuditQuery = computed(() => `${selectedScope.value}:${didFilter.value.trim()}`)
const hasExecutedAudit = computed(() => successfulAuditQueries.value[selectedScope.value] === currentAuditQuery.value)
const findings = computed(() => (hasExecutedAudit.value ? (auditFindingsByScope.value[selectedScope.value] ?? []) : []))
const auditLoading = computed(() => auditLoadingScope.value !== null)
const reportLoading = computed(() => reportLoadingScope.value !== null)
const selectedAuditLoading = computed(() => auditLoadingScope.value === selectedScope.value)
const selectedReportLoading = computed(() => reportLoadingScope.value === selectedScope.value)

const filteredFindings = computed(() => {
  return findings.value.filter((finding) => {
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
watch(filteredFindings, (visibleFindings) => {
  if (
    selectedFindingId.value !== null &&
    !visibleFindings.some((finding) => String(finding.id) === String(selectedFindingId.value))
  ) {
    selectedFindingId.value = null
  }
})
const selectedFindingDetailRows = computed(() => {
  const finding = selectedFinding.value
  if (!finding) {
    return []
  }
  const details = findingDetails(finding)
  return [
    { label: 'Checked', value: stringDetail(checkAssertion(finding)) },
    { label: 'Rule ID', value: stringDetail(details?.rule_id ?? details?.ruleId) },
    { label: 'Policy Set', value: stringDetail(details?.policySetId) },
    { label: 'Policy Version', value: stringDetail(details?.policyVersion) },
    { label: 'Requirement', value: detailValue(details?.requirement) },
    { label: 'Actual value', value: detailValue(details?.actualValue) },
    { label: 'Expected value', value: detailValue(details?.expectedValue) },
    { label: 'Expected values', value: detailValue(details?.expectedValues) },
    { label: 'Operator', value: detailValue(details?.operator) },
    { label: 'Field IRI', value: stringDetail(details?.fieldIri) },
    { label: 'Path', value: stringDetail(details?.path) },
    { label: 'Ontology Term', value: stringDetail(details?.ontologyTerm) },
    { label: 'Object Type', value: stringDetail(details?.objectType ?? finding.object_type) },
    { label: 'Contract Version', value: stringDetail(details?.contractVersion) },
    { label: 'Audited By', value: stringDetail(details?.auditedBy) },
    { label: 'Component', value: stringDetail(finding.component) },
    { label: 'DID', value: stringDetail(finding.did) },
  ].filter((row) => row.value !== emptyValueLabel)
})

const failedCheckCount = computed(() =>
  hasExecutedAudit.value ? findings.value.filter((finding) => auditResult(finding) === 'failed').length : 0,
)
const passedCheckCount = computed(() =>
  hasExecutedAudit.value ? findings.value.filter((finding) => auditResult(finding) === 'passed').length : 0,
)
const reviewCheckCount = computed(() =>
  hasExecutedAudit.value ? findings.value.filter((finding) => auditResult(finding) === 'review').length : 0,
)
const notEvaluatedCheckCount = computed(() =>
  hasExecutedAudit.value ? findings.value.filter((finding) => auditResult(finding) === 'not_evaluated').length : 0,
)
const auditIsEmpty = computed(() => hasExecutedAudit.value && findings.value.length === 0 && !error.value)
const auditHasPassed = computed(
  () =>
    hasExecutedAudit.value &&
    findings.value.length > 0 &&
    passedCheckCount.value === findings.value.length &&
    !error.value,
)
const tableFilterGroups: { key: TableFilterKey; label: string }[] = [
  { key: 'result', label: 'Status' },
  { key: 'category', label: 'Finding' },
  { key: 'component', label: 'Component' },
  { key: 'did', label: 'DID' },
]
const tableFilterOptions = computed<Record<TableFilterKey, string[]>>(() => ({
  result: uniqueTableValues(findings.value.map((finding) => auditResultLabel(finding))),
  category: uniqueTableValues(findings.value.map((finding) => finding.category)),
  component: uniqueTableValues(findings.value.map((finding) => finding.component)),
  did: uniqueTableValues(findings.value.map((finding) => finding.did)),
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

watch(currentAuditQuery, () => {
  reportError.value = ''
})

watch(selectedScope, () => {
  selectedFindingId.value = null
})

// DCS-FR-CSA-24: the Auditor reads the contract itself from this view. The
// read is an explicit action because the backend audits every retrieval.
const isAuditor = computed(() => authStore.user?.roles.includes('AUDITOR') ?? false)
const inspectedContract = ref<Awaited<ReturnType<typeof contractWorkflowService.retrieveById>>>(null)
const contractLoading = ref(false)
const contractNotFound = ref(false)
watch(didFilter, () => {
  inspectedContract.value = null
  contractNotFound.value = false
})

async function loadInspectedContract() {
  contractLoading.value = true
  try {
    inspectedContract.value = await contractWorkflowService.retrieveById({ did: didFilter.value.trim() })
    contractNotFound.value = inspectedContract.value === null
  } finally {
    contractLoading.value = false
  }
}

function exportInspectedContract() {
  const contract = inspectedContract.value
  if (!contract) return
  saveBlob(new Blob([JSON.stringify(contract, null, 2)], { type: 'application/json' }), 'contract.json')
}

// Key-erasure state of the inspected archive entry (DCS-NFR-SEC-13): shown
// whenever the archive scope is filtered to one contract DID, so the entry's
// "Keys destroyed" record stays visible after the entry itself left the
// archive listing.
const erasureStatus = ref<ArchiveErasureStatus | null>(null)
watchDebounced(
  [selectedScope, didFilter],
  async ([scope, did]) => {
    erasureStatus.value = null
    if (scope !== 'archive' || !did.trim()) return
    try {
      erasureStatus.value = await archiveService.erasureStatus(did.trim())
    } catch {
      // no erasure record to show — the panel simply stays hidden
    }
  },
  { debounce: 400, immediate: true },
)

const executeAudit = async () => {
  reportError.value = ''
  const scope = selectedScope.value
  const query = currentAuditQuery.value
  auditLoadingScope.value = scope
  auditErrorsByScope.value = { ...auditErrorsByScope.value, [scope]: undefined }
  successfulAuditQueries.value = { ...successfulAuditQueries.value, [scope]: undefined }
  auditFindingsByScope.value = { ...auditFindingsByScope.value, [scope]: [] }
  selectedFindingId.value = null
  try {
    const scopeFindings = await auditingService.audit({
      scope,
      did: didFilter.value.trim() || undefined,
      justification: justification.value.trim(),
    })
    auditFindingsByScope.value = { ...auditFindingsByScope.value, [scope]: scopeFindings }
    successfulAuditQueries.value = { ...successfulAuditQueries.value, [scope]: query }
    selectedFindingId.value = null
  } catch (err) {
    console.error('Audit Error:', err)
    // The failure is rendered in place, per scope, so the interceptor's toast
    // for it is dropped rather than announced a second time.
    claimReportedHttpError(err)
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
  const query = currentAuditQuery.value
  reportError.value = ''
  reportLoadingScope.value = scope
  reportLoadingFormat.value = format
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
    claimReportedHttpError(err)
    if (query === currentAuditQuery.value) reportError.value = 'Audit report could not be generated.'
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

function findingBadgeClass(finding: AuditFinding): 'badge-success' | 'badge-error' | 'badge-warning' | 'badge-ghost' {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'badge-success'
  }
  if (result === 'failed') {
    return 'badge-error'
  }
  // A not-evaluated finding shares the neutral badge with an unknown one: it is
  // neither a pass nor a review task (ADR-33).
  return result === 'review' ? 'badge-warning' : 'badge-ghost'
}

function auditResult(finding: AuditFinding): AuditResult | null {
  if (finding.status === 'PASSED') {
    return 'passed'
  }
  if (finding.status === 'FAILED') {
    return 'failed'
  }
  if (finding.status === 'REVIEW') {
    return 'review'
  }
  if (finding.status === 'NOT_EVALUATED') {
    return 'not_evaluated'
  }
  return null
}

function auditResultLabel(finding: AuditFinding): 'Passed' | 'Failed' | 'Needs Review' | 'Not Evaluated' | 'Unknown' {
  const result = auditResult(finding)
  if (result === 'passed') {
    return 'Passed'
  }
  if (result === 'failed') {
    return 'Failed'
  }
  if (result === 'not_evaluated') {
    return 'Not Evaluated'
  }
  return result === 'review' ? 'Needs Review' : 'Unknown'
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
  if (result === 'review') {
    return assertion ? `Review: ${assertion}` : 'Review required'
  }
  if (result === 'not_evaluated') {
    return assertion
      ? `Not evaluated here: ${assertion}`
      : 'Not evaluated here - the executing target system reports this verdict'
  }
  return assertion ? `Unknown result: ${assertion}` : 'Unknown result'
}

function checkAssertion(finding: AuditFinding): string {
  const details = findingDetails(finding)
  const message = stringDetail(details?.message ?? details?.reason)
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
  return stringDetail(findingDetails(finding)?.severity)
}

function severityBadgeClass(finding: AuditFinding): 'badge-success' | 'badge-error' | 'badge-warning' | 'badge-ghost' {
  const severityValue = findingDetails(finding)?.severity
  const severity = typeof severityValue === 'string' ? severityValue.trim().toLowerCase() : ''
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
    severity === 'satisfied' ||
    severity === 'passed' ||
    severity === 'pass' ||
    severity === 'success' ||
    severity === 'ok' ||
    severity === 'compliant'
  ) {
    return 'badge-success'
  }
  // 'deferred' falls through to the neutral badge: the DCS reached no verdict,
  // so it must not read as a passing check (ADR-33).
  return 'badge-ghost'
}

function findingDetails(finding: AuditFinding): Record<string, unknown> | null {
  if (!isObjectRecord(finding.details)) {
    return null
  }
  if (isObjectRecord(finding.details.finding)) {
    return finding.details.finding
  }
  return finding.details
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
  <div class="mb-4 border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
    <div v-if="showAuditTabList" role="tablist" class="tabs-border mt-4 tabs">
      <button
        type="button"
        role="tab"
        class="tab"
        :class="{ 'tab-active': activeTab === 'compliance' }"
        :aria-selected="activeTab === 'compliance'"
        @click="setActiveTab('compliance')"
      >
        Compliance Audit
      </button>
      <button
        type="button"
        role="tab"
        class="tab"
        :class="{ 'tab-active': activeTab === 'action-history' }"
        :aria-selected="activeTab === 'action-history'"
        @click="setActiveTab('action-history')"
      >
        Action History
      </button>
    </div>
  </div>

  <section v-if="activeTab === 'action-history' && canSeeActionHistory" class="space-y-4 px-4 pb-8">
    <section
      v-if="canExportReportOnly"
      class="space-y-3 rounded-box border border-base-content/10 bg-base-100 p-4"
      data-testid="audit-report-export"
    >
      <h3 class="font-bold">Audit report</h3>
      <p class="text-sm opacity-70">Export the latest audit run of the contracts or the archive.</p>
      <div class="grid min-w-0 gap-3 sm:grid-cols-3">
        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">Scope</span>
          <select id="select-report-scope" v-model="selectedScope" class="select-bordered select w-full rounded-box">
            <option v-for="scope in reportOnlyScopeOptions" :key="scope.value" :value="scope.value">
              {{ scope.label }}
            </option>
          </select>
        </label>
        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">DID (optional)</span>
          <input id="input-report-did" v-model="didFilter" class="input-bordered input w-full rounded-box" />
        </label>
        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">Report justification</span>
          <input
            id="input-report-justification"
            v-model="justification"
            class="input-bordered input w-full rounded-box"
          />
        </label>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="format in ['json', 'csv', 'pdf'] as const"
          :key="format"
          class="btn rounded-box btn-outline btn-sm"
          :disabled="reportLoading || !justification.trim()"
          :data-testid="`audit-report-export-${format}`"
          @click="generateReport(format)"
        >
          {{ format.toUpperCase() }}
        </button>
      </div>
      <p v-if="reportError" role="alert" class="text-sm text-error">{{ reportError }}</p>
    </section>
    <ActionHistoryPanel :initial-did="didFilter" />
  </section>

  <section v-if="activeTab === 'compliance' && canSeeComplianceAudit" class="space-y-4 px-4">
    <section
      class="space-y-3 rounded-box border border-base-content/10 bg-base-100 p-4"
      aria-labelledby="audit-criteria-title"
    >
      <h3 id="audit-criteria-title" class="font-bold">Audit criteria</h3>
      <div class="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_minmax(0,1.6fr)_auto]">
        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">Scope</span>
          <select
            id="select-scope"
            v-model="selectedScope"
            class="select-bordered select w-full rounded-box"
            :disabled="auditLoading || reportLoading"
          >
            <option v-for="scope in scopeOptions" :key="scope.value" :value="scope.value">
              {{ scope.label }}
            </option>
          </select>
        </label>

        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">DID (optional)</span>
          <input
            id="input-did"
            v-model="didFilter"
            class="input-bordered input w-full rounded-box"
            :disabled="auditLoading || reportLoading"
          />
        </label>

        <label class="form-control flex min-w-0 flex-col">
          <span class="label-text mb-1">Audit justification</span>
          <input
            id="input-justification"
            v-model="justification"
            required
            class="input-bordered input w-full rounded-box"
            :disabled="auditLoading || reportLoading"
          />
        </label>

        <button
          class="btn relative rounded-box btn-primary sm:self-end"
          :disabled="auditLoading || reportLoading || !justification.trim()"
          @click="executeAudit"
        >
          <span
            v-if="selectedAuditLoading"
            aria-hidden="true"
            class="loading absolute left-3 loading-sm loading-spinner"
          ></span>
          <span class="px-4">Execute Audit</span>
        </button>
      </div>
    </section>
    <section
      class="space-y-3 rounded-box border border-base-content/10 bg-base-100 p-4"
      aria-labelledby="audit-result-title"
    >
      <h3 id="audit-result-title" class="font-bold">Audit result</h3>
      <div class="grid min-w-0 gap-4 2xl:grid-cols-[minmax(0,1fr)_auto]">
        <div class="grid min-w-0 grid-cols-2 gap-4 lg:grid-cols-4" data-testid="audit-counters">
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="size-2 rounded-full bg-error/50"></span>
              Failed Checks
            </div>
            <div class="text-2xl font-semibold text-base-content">{{ hasExecutedAudit ? failedCheckCount : '—' }}</div>
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="size-2 rounded-full bg-success/50"></span>
              Passed Checks
            </div>
            <div class="text-2xl font-semibold text-base-content">{{ hasExecutedAudit ? passedCheckCount : '—' }}</div>
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="size-2 rounded-full bg-warning/50"></span>
              Needs Review
            </div>
            <div class="text-2xl font-semibold text-base-content">{{ hasExecutedAudit ? reviewCheckCount : '—' }}</div>
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="size-2 rounded-full bg-base-content/30"></span>
              Not Evaluated
            </div>
            <div class="text-2xl font-semibold text-base-content">
              {{ hasExecutedAudit ? notEvaluatedCheckCount : '—' }}
            </div>
          </div>
        </div>

        <fieldset class="min-w-0 space-y-2">
          <legend class="text-sm font-medium">Download report</legend>
          <div class="flex flex-nowrap gap-2">
            <button
              class="btn relative rounded-box whitespace-nowrap btn-outline"
              :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
              @click="generateReport('json')"
            >
              <span
                v-if="selectedReportLoading && reportLoadingFormat === 'json'"
                aria-hidden="true"
                class="loading absolute left-2 loading-sm loading-spinner"
              ></span>
              <span class="px-3">JSON</span>
            </button>
            <button
              class="btn relative rounded-box whitespace-nowrap btn-outline"
              :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
              @click="generateReport('csv')"
            >
              <span
                v-if="selectedReportLoading && reportLoadingFormat === 'csv'"
                aria-hidden="true"
                class="loading absolute left-2 loading-sm loading-spinner"
              ></span>
              <span class="px-3">CSV</span>
            </button>
            <button
              class="btn relative rounded-box whitespace-nowrap btn-outline"
              :disabled="reportLoading || auditLoading || !hasExecutedAudit || !justification.trim()"
              @click="generateReport('pdf')"
            >
              <span
                v-if="selectedReportLoading && reportLoadingFormat === 'pdf'"
                aria-hidden="true"
                class="loading absolute left-2 loading-sm loading-spinner"
              ></span>
              <span class="px-3">PDF</span>
            </button>
          </div>
          <p v-if="selectedReportLoading" role="status" class="text-sm">
            Preparing {{ reportLoadingFormat?.toUpperCase() }} report…
          </p>
          <p v-if="reportError" role="alert" class="text-sm text-error">{{ reportError }}</p>
        </fieldset>
      </div>
    </section>

    <div v-if="selectedAuditLoading" class="p-4" role="status">Executing audit...</div>
    <div v-else-if="error" class="alert rounded-box alert-error" role="alert">{{ error }}</div>
    <div v-if="auditHasPassed" class="alert rounded-box alert-success" role="status">
      Audit passed. Every check returned a verdict, and none failed or needs review.
    </div>
    <div v-if="notEvaluatedCheckCount > 0" class="alert rounded-box alert-info" role="status">
      {{ notEvaluatedCheckCount }} check(s) were not evaluated here. Their verdict belongs to the target system that
      executes the contract, which observes the facts they depend on.
    </div>
    <div v-if="auditIsEmpty" class="alert rounded-box alert-info" role="status">
      Audit completed. No matching entries were found.
    </div>

    <!-- Contract content for the Auditor (DCS-FR-CSA-24) -->
    <section
      v-if="isAuditor && didFilter.trim()"
      class="rounded-box border border-base-content/10 bg-base-100 p-4"
      data-testid="audit-contract"
    >
      <div class="flex flex-wrap items-center gap-3">
        <h3 class="font-bold">Contract</h3>
        <button
          class="btn btn-sm"
          :disabled="contractLoading"
          data-testid="audit-contract-load"
          @click="loadInspectedContract"
        >
          Show contract
        </button>
        <button v-if="inspectedContract" class="btn btn-ghost btn-sm" @click="exportInspectedContract">
          Export JSON
        </button>
      </div>
      <div v-if="inspectedContract" class="mt-2 space-y-2 text-sm" data-testid="audit-contract-details">
        <div>
          <span class="font-medium">{{ inspectedContract.name || inspectedContract.did }}</span>
          · version {{ inspectedContract.contract_version }} · {{ inspectedContract.state }}
        </div>
        <div class="opacity-70">
          Created {{ formatDateTime(inspectedContract.created_at) }} by {{ inspectedContract.created_by }}
          <template v-if="inspectedContract.start_date || inspectedContract.exp_date">
            · valid {{ formatDateTime(inspectedContract.start_date) }} to
            {{ formatDateTime(inspectedContract.exp_date) }}
          </template>
        </div>
        <pre class="max-h-96 overflow-auto rounded-box bg-base-200 p-2 text-xs">{{
          JSON.stringify(inspectedContract.contract_data, null, 2)
        }}</pre>
      </div>
      <p v-else-if="contractNotFound" class="mt-2 text-sm opacity-70">No contract with this DID can be read.</p>
    </section>

    <!-- Erasure state of the inspected archive entry (DCS-NFR-SEC-13) -->
    <section
      v-if="erasureStatus"
      class="rounded-box border border-base-content/10 bg-base-100 p-4"
      data-testid="erasure-status"
    >
      <div class="flex flex-wrap items-center gap-3">
        <h3 class="font-bold">Encryption keys</h3>
        <span
          v-if="erasureStatus.local_status === 'shredded'"
          class="badge badge-error"
          data-testid="erasure-status-badge"
        >
          Keys destroyed
        </span>
        <span v-else class="badge badge-ghost" data-testid="erasure-status-badge">Keys live</span>
      </div>
      <details class="collapse-arrow collapse mt-2 rounded-box border border-base-content/10">
        <summary class="collapse-title text-sm font-medium">Erasure details</summary>
        <div class="collapse-content space-y-3 text-sm" data-testid="erasure-status-details">
          <div v-if="erasureStatus.local_status === 'shredded'">
            Destroyed {{ formatDateTime(erasureStatus.shredded_at) }} by {{ erasureStatus.shredded_by }}:
            {{ erasureStatus.shred_reason }}
          </div>
          <div v-else>The local encryption keys are live. The archived content is decryptable.</div>
          <p v-if="erasureStatus.peers.length === 0" class="opacity-70">No counterparty instance holds keys.</p>
          <table v-else class="table table-xs">
            <thead>
              <tr>
                <th>Peer</th>
                <th>Status</th>
                <th>Requested</th>
                <th>Confirmed</th>
                <th>Retries</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="peer in erasureStatus.peers"
                :key="peer.peer_did"
                :data-testid="`erasure-peer-${peer.peer_did}`"
              >
                <td class="font-mono text-xs">{{ peer.peer_did }}</td>
                <td>
                  <span class="badge badge-sm" :class="peer.status === 'confirmed' ? 'badge-success' : 'badge-warning'">
                    {{ peer.status }}
                  </span>
                </td>
                <td>{{ formatDateTime(peer.requested_at) }}</td>
                <td>{{ peer.confirmed_at ? formatDateTime(peer.confirmed_at) : '—' }}</td>
                <td>
                  {{ peer.retry_count }}
                  <span v-if="peer.last_tried_at" class="opacity-70">
                    (last tried {{ formatDateTime(peer.last_tried_at) }})
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </details>
    </section>

    <div
      v-if="!selectedAuditLoading && !error"
      class="grid min-w-0 gap-4"
      :class="{ 'xl:grid-cols-[minmax(0,1fr)_24rem]': selectedFinding }"
    >
      <div class="min-w-0 rounded-box border border-base-content/10">
        <div class="space-y-3 border-b border-base-content/10 px-4 py-3">
          <h3 class="font-bold">Findings</h3>
          <div v-if="findings.length > 0" class="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center">
            <span class="col-span-2 w-full text-sm font-medium opacity-70">Table filters</span>
            <details
              v-for="(group, index) in tableFilterGroups"
              :key="group.key"
              class="dropdown"
              :class="index % 2 === 0 ? 'dropdown-start' : 'dropdown-end sm:dropdown-start'"
            >
              <summary class="btn rounded-box btn-outline btn-sm">
                {{ group.label }}
                <span class="badge badge-sm">
                  {{ checkedTableFilterCount(group.key) }}/{{ tableFilterOptions[group.key].length }}
                </span>
              </summary>
              <div
                class="dropdown-content z-10 mt-2 w-64 max-w-[calc(100vw-4rem)] rounded-box border border-base-content/10 bg-base-100 p-3 shadow"
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
        <div v-if="findings.length > 0" class="overflow-x-auto" data-testid="audit-findings-scroll">
          <table class="table table-zebra">
            <thead>
              <tr class="text-base-content/70">
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
                <td colspan="3" class="py-8 text-center opacity-70">No findings match the selected filters.</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else-if="!hasExecutedAudit" class="p-4 text-sm opacity-70">Select a scope and execute an audit.</p>
      </div>

      <aside
        v-if="selectedFinding"
        class="rounded-box border border-base-content/10 bg-base-100 xl:sticky xl:top-4 xl:self-start"
      >
        <div class="flex items-center justify-between border-b border-base-content/10 px-4 py-3">
          <h3 class="font-bold">Finding Details</h3>
          <button type="button" class="btn btn-ghost btn-xs" @click="selectedFindingId = null">Close</button>
        </div>
        <div class="space-y-4 p-4">
          <div>
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

          <p class="text-sm wrap-break-word whitespace-pre-wrap opacity-80">{{ selectedFinding.description }}</p>

          <dl class="divide-y divide-base-content/10 text-sm">
            <div
              v-for="row in selectedFindingDetailRows"
              :key="row.label"
              class="grid grid-cols-[8rem_minmax(0,1fr)] gap-3 py-2"
            >
              <dt class="font-medium opacity-70">{{ row.label }}</dt>
              <dd class="wrap-break-word">{{ row.value }}</dd>
            </div>
          </dl>

          <details
            v-if="selectedFindingRawDetails"
            class="collapse-arrow collapse rounded-box border border-base-content/10"
          >
            <summary class="collapse-title text-sm font-medium">Raw Details</summary>
            <div class="collapse-content">
              <pre class="text-xs wrap-break-word whitespace-pre-wrap">{{ selectedFindingRawDetails }}</pre>
            </div>
          </details>
        </div>
      </aside>
    </div>
    <CheckpointProofPanel v-if="isAuditor" />
    <AccessAttempts />
  </section>
</template>
