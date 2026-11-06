<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  type SignatureClauseReportResult,
  signatureManagementService,
  type SignatureViewItem,
} from '@/services/signature-management-service'
import {
  buildClauseComplianceReport,
  type ClauseComplianceReport,
  parseApplicableChecks,
} from '@/utils/clause-compliance-report'
import { buildPartyComplianceReport, type PartyComplianceReport } from '@/utils/party-compliance-report'
import { findingIndicator } from '@/utils/signature-verdict'

const props = defineProps<{
  contractDid: string
  signatures: SignatureViewItem[]
  complianceFindings: string[]
  integrityFindings: string[]
}>()

const emit = defineEmits<{
  reportReady: [
    payload: {
      party: PartyComplianceReport
      clause: ClauseComplianceReport
    },
  ]
}>()

type ReportTab = 'party' | 'clauses'

const reportTab = ref<ReportTab>('party')

const partyReport = computed(() =>
  buildPartyComplianceReport(props.signatures, props.complianceFindings, props.integrityFindings),
)

const clauseLoading = ref(false)
const clauseError = ref<string | null>(null)
const clauseApi = ref<SignatureClauseReportResult | null>(null)

const clauseReport = computed((): ClauseComplianceReport => {
  const api = clauseApi.value
  if (!api) {
    return buildClauseComplianceReport({
      clauses: [],
      findings: [],
      clausesAvailable: false,
      findingsAvailable: false,
      findingsUnavailableReason:
        'Open By Clauses to load the live SHACL/content-policy projection (GET /signature/clause-report).',
    })
  }
  // Prefer server sections (already mapped); rebuild display from API status.
  return {
    scope_note: api.scope_note ?? '',
    clauses: api.clauses.map((c) => ({
      clause_id: c.clause_id,
      title: c.title ?? c.clause_id,
      findings: c.findings.map((f) => ({
        rule_id: f.rule_id,
        severity: f.severity,
        message: f.message,
        path: f.path,
        field_iri: f.field_iri,
        result: f.severity,
      })),
      non_conformance_summary: c.findings.length
        ? c.findings.map((f) => f.message).join('; ')
        : 'No clause-specific findings',
      assessment: c.verdict_note?.trim() ? c.verdict_note.trim() : clauseStatusLabel(c.status),
      status: c.status,
      assessment_run: c.assessment_run,
      applicable_checks: c.applicable_checks,
      verdict_note: c.verdict_note,
    })),
    unassigned_findings: api.unassigned_findings.map((f) => ({
      rule_id: f.rule_id,
      severity: f.severity,
      message: f.message,
      path: f.path,
      field_iri: f.field_iri,
      result: f.severity,
    })),
    clauses_available: true,
    findings_available: true,
    clauses_unavailable_reason: null,
    findings_unavailable_reason: null,
  }
})

function clauseStatusLabel(status: string): string {
  switch (status) {
    case 'issues_identified':
      return 'Issues identified from attributed content-policy findings'
    case 'warnings_only':
      return 'Warnings only from attributed content-policy findings'
    case 'compliant':
      return 'Applicable clause checks ran without attributed violations'
    case 'undetermined':
      return 'Available validation results do not establish a clause-level compliance verdict'
    case 'not_assessed':
    default:
      return 'No applicable clause-level checks were established for this clause'
  }
}

function clauseStatusBadge(status: string): { label: string; cls: string } {
  switch (status) {
    case 'issues_identified':
      return { label: 'Issues', cls: 'badge-error' }
    case 'warnings_only':
      return { label: 'Warnings', cls: 'badge-warning' }
    case 'compliant':
      return { label: 'Compliant', cls: 'badge-success' }
    case 'undetermined':
      return { label: 'Undetermined', cls: 'badge-ghost' }
    default:
      return { label: 'Not assessed', cls: 'badge-ghost' }
  }
}

watch(
  [partyReport, clauseReport],
  () => {
    emit('reportReady', { party: partyReport.value, clause: clauseReport.value })
  },
  { immediate: true },
)

function formatSignedAt(iso: string | undefined): string {
  if (!iso?.trim()) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  // e.g. 15 Oct 2026, 08:30 UTC
  const day = d.getUTCDate()
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  const mon = months[d.getUTCMonth()]
  const year = d.getUTCFullYear()
  const hh = String(d.getUTCHours()).padStart(2, '0')
  const mm = String(d.getUTCMinutes()).padStart(2, '0')
  return `${day} ${mon} ${year}, ${hh}:${mm} UTC`
}

async function loadClauseReport() {
  clauseError.value = null
  clauseLoading.value = true
  try {
    clauseApi.value = await signatureManagementService.getClauseReport(props.contractDid)
  } catch (e: unknown) {
    clauseApi.value = null
    clauseError.value = e instanceof Error ? e.message : String(e)
  } finally {
    clauseLoading.value = false
  }
}

watch(
  () => props.contractDid,
  () => {
    clauseApi.value = null
    if (reportTab.value === 'clauses') void loadClauseReport()
  },
)

watch(reportTab, (tab) => {
  if (tab === 'clauses' && !clauseApi.value && !clauseLoading.value) {
    void loadClauseReport()
  }
})
</script>

<template>
  <section data-testid="compliance-report-panel" class="space-y-3">
    <div>
      <h3 class="font-semibold">Compliance Report</h3>
      <p class="text-sm text-base-content/60">Compliance findings organized by contract party or clause.</p>
    </div>

    <div role="tablist" class="tabs-boxed tabs tabs-sm">
      <button
        role="tab"
        class="tab"
        :class="{ 'tab-active': reportTab === 'party' }"
        data-testid="report-tab-party"
        @click="reportTab = 'party'"
      >
        By Party
      </button>
      <button
        role="tab"
        class="tab"
        :class="{ 'tab-active': reportTab === 'clauses' }"
        data-testid="report-tab-clauses"
        @click="reportTab = 'clauses'"
      >
        By Clauses
      </button>
    </div>

    <div v-if="reportTab === 'party'" data-testid="party-compliance-report" class="space-y-3">
      <p class="text-xs text-base-content/50">{{ partyReport.scope_note }}</p>

      <div
        v-for="party in partyReport.parties"
        :key="`${party.signer_did}-${party.field_name ?? ''}`"
        data-testid="party-compliance-section"
        class="card border border-base-content/10 bg-base-100"
      >
        <div class="card-body gap-2 p-4 text-sm">
          <dl class="space-y-1">
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Signature status</dt>
              <dd class="m-0 text-base-content/80">
                <span class="badge badge-sm" :class="party.status_cls">{{ party.status_label }}</span>
              </dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Signer</dt>
              <dd class="m-0 min-w-0 break-all text-base-content/80">{{ party.signer }}</dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Signing level</dt>
              <dd class="m-0 text-base-content/80">{{ party.credential_type }}</dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Authorized role</dt>
              <dd class="m-0 text-base-content/80">{{ party.authorized_role ?? '—' }}</dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Signed at</dt>
              <dd class="m-0 text-base-content/80">{{ formatSignedAt(party.signed_at) }}</dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Non-conformance</dt>
              <dd class="m-0 text-base-content/80">{{ party.non_conformance_summary }}</dd>
            </div>
          </dl>
        </div>
      </div>

      <p v-if="!partyReport.parties.length" class="text-sm text-base-content/60">
        No local signing evidence on this instance.
      </p>

      <div v-if="partyReport.contract_findings.length" class="space-y-1">
        <h4 class="font-medium">Contract-level findings</h4>
        <ul class="space-y-1 text-sm">
          <li
            v-for="(f, i) in partyReport.contract_findings"
            :key="`contract-finding-${i}`"
            class="flex items-start gap-2"
          >
            <span class="badge badge-sm" :class="findingIndicator(f).cls">
              {{ findingIndicator(f).label }}
            </span>
            <span>{{ f }}</span>
          </li>
        </ul>
      </div>
    </div>

    <div v-else data-testid="clause-compliance-report" class="space-y-3">
      <p v-if="clauseApi?.scope_note" class="text-xs text-base-content/50">{{ clauseApi.scope_note }}</p>
      <div v-if="clauseError" class="alert text-sm alert-error">{{ clauseError }}</div>
      <div v-if="clauseLoading" class="text-sm text-base-content/60" role="status">
        Loading clause compliance projection…
      </div>

      <div
        v-for="clause in clauseApi?.clauses ?? []"
        :key="clause.clause_id"
        data-testid="clause-compliance-section"
        class="card border border-base-content/10 bg-base-100"
      >
        <div class="card-body gap-2 p-4 text-sm">
          <h4 class="m-0 font-medium break-all">{{ clause.title ?? 'Untitled clause' }}</h4>
          <dl class="space-y-1">
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Status</dt>
              <dd class="m-0 text-base-content/80">
                <span class="badge badge-sm" :class="clauseStatusBadge(clause.status).cls">
                  {{ clauseStatusBadge(clause.status).label }}
                </span>
              </dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Assessment run</dt>
              <dd class="m-0 text-base-content/80">{{ formatSignedAt(clause.assessment_run) }}</dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Applicable checks</dt>
              <dd class="m-0 text-base-content/80">
                <ul
                  class="m-0 flex list-none flex-wrap items-center gap-y-1 p-0"
                  data-testid="clause-applicable-checks"
                >
                  <li
                    v-for="(check, ci) in parseApplicableChecks(clause.applicable_checks)"
                    :key="`${check.label}-${ci}`"
                    class="inline-flex items-center"
                  >
                    <span v-if="ci > 0" class="mx-2 text-base-content/30" aria-hidden="true">·</span>
                    <span
                      :class="check.id ? 'border-b border-dotted border-base-content/40' : undefined"
                      :title="check.id"
                      :aria-label="check.id ? `${check.label}: ${check.id}` : check.label"
                    >
                      {{ check.label }}
                    </span>
                  </li>
                </ul>
              </dd>
            </div>
            <div class="flex items-baseline gap-x-3">
              <dt class="w-40 shrink-0 text-base-content/60">Non-conformance</dt>
              <dd class="m-0 text-base-content/80">
                <template v-if="clause.findings.length">
                  <ul class="m-0 list-disc space-y-1 pl-4">
                    <li v-for="(f, i) in clause.findings" :key="i">
                      {{ f.message }}
                    </li>
                  </ul>
                </template>
                <template v-else>No clause-specific findings</template>
              </dd>
            </div>
          </dl>
          <p v-if="clause.verdict_note" class="m-0 text-xs text-base-content/60">
            {{ clause.verdict_note }}
          </p>
        </div>
      </div>

      <p v-if="clauseApi && !clauseApi.clauses.length && !clauseLoading" class="text-sm text-base-content/60">
        No dcs:Clause blocks found in the contract document.
      </p>

      <div v-if="clauseApi?.unassigned_findings?.length" class="space-y-1">
        <h4 class="font-medium">Unassigned content findings</h4>
        <ul class="space-y-1 text-sm">
          <li v-for="(f, i) in clauseApi.unassigned_findings" :key="`unassigned-${i}`">
            {{ f.message }}
            <span v-if="f.focus_node" class="text-base-content/50">(focus: {{ f.focus_node }})</span>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>
