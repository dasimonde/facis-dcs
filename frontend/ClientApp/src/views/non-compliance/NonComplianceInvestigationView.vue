<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ROUTES } from '@/router/router'
import { pacNonComplianceService } from '@/services/pac-non-compliance-service'
import { useAuthStore } from '@/stores/auth-store'
import { downloadBlob } from '@/utils/download-blob'
import { claimReportedHttpError } from '@/utils/report-action-error'
import type { PACCase, PACComplianceRisk, PACWorkflowGateRun } from '@/models/responses/pac-non-compliance-response'

// The Compliance Officer monitors, reports incidents and decides workflow-gate
// reviews. The Auditor reads and exports the case files (DCS-FR-PACM-05).
const authStore = useAuthStore()
const isComplianceOfficer = computed(() => authStore.user?.roles.includes('COMPLIANCE_OFFICER') ?? false)

const risks = ref<PACComplianceRisk[]>([])
const checkedAt = ref<string | null>(null)
const sweepLoading = ref(false)
const sweepError = ref<string | null>(null)
const searchTerm = ref('')

const filteredRisks = computed(() => {
  const term = searchTerm.value.trim().toLowerCase()
  if (!term) {
    return risks.value
  }
  return risks.value.filter((risk) => risk.did.toLowerCase().includes(term))
})
const hasCompletedSweep = computed(() => checkedAt.value !== null)
const hasActiveFilter = computed(() => searchTerm.value.trim() !== '')

// Optional: with it, the risks name the denied actors (DCS-FR-PACM-04).
const monitorJustification = ref('')

const runMonitoringSweep = async () => {
  sweepLoading.value = true
  sweepError.value = null
  try {
    const response = await pacNonComplianceService.monitor(monitorJustification.value.trim() || undefined)
    checkedAt.value = response.checked_at
    risks.value = response.risks
  } catch (err) {
    claimReportedHttpError(err)
    sweepError.value = err instanceof Error ? err.message : 'Monitoring sweep could not be executed.'
  } finally {
    sweepLoading.value = false
  }
}

// A contract in force that no longer meets its own agreed boundaries is the
// one risk here that is about the contract UNDERPERFORMING rather than about a
// process step being missed, so it is called out visually (DCS-FR-CWE-31).
const isUnderperformance = (risk: PACComplianceRisk) => risk.risk_type === 'CONTRACT_UNDERPERFORMANCE'

// DCS-IR-PACM-04: an incident is linked to a contract or to a template.
const linkedTo = ref<'contract' | 'template'>('contract')
const linkedDid = ref('')
const riskType = ref('')
const detail = ref('')
const incidentSubmitting = ref(false)
const incidentError = ref<string | null>(null)
const incidentSuccess = ref(false)
const caseId = ref<string | null>(null)

const canSubmitIncident = computed(
  () => linkedDid.value.trim() !== '' && riskType.value.trim() !== '' && detail.value.trim() !== '',
)

const submitIncidentReport = async () => {
  incidentSubmitting.value = true
  incidentError.value = null
  incidentSuccess.value = false
  try {
    const did = linkedDid.value.trim()
    caseId.value =
      (await pacNonComplianceService.reportIncident({
        ...(linkedTo.value === 'contract' ? { contract_did: did } : { template_did: did }),
        findings: [{ risk_type: riskType.value.trim(), detail: detail.value.trim() }],
      })) ?? null
    incidentSuccess.value = true
    if (caseId.value) caseLookup.value = caseId.value
    await loadCases()
  } catch (err) {
    claimReportedHttpError(err)
    incidentError.value = err instanceof Error ? err.message : 'Incident report could not be submitted.'
  } finally {
    incidentSubmitting.value = false
  }
}

// DCS-FR-PACM-05: the case list, the case lookup and the case file export.
const cases = ref<PACCase[]>([])
const casesError = ref<string | null>(null)
const caseLookup = ref('')
const caseJustification = ref('')
const caseExporting = ref(false)

async function loadCases() {
  casesError.value = null
  try {
    cases.value = await pacNonComplianceService.cases()
  } catch (err) {
    claimReportedHttpError(err)
    casesError.value = err instanceof Error ? err.message : 'Cases could not be loaded.'
  }
}

const exportCaseFile = async (id: string) => {
  if (!id.trim() || !caseJustification.value.trim()) return
  caseExporting.value = true
  casesError.value = null
  try {
    const file = await pacNonComplianceService.exportCaseFile(id.trim(), caseJustification.value.trim())
    downloadBlob(file, `case-${id.trim()}.json`)
    await loadCases()
  } catch (err) {
    claimReportedHttpError(err)
    casesError.value = err instanceof Error ? err.message : 'Case file could not be exported.'
  } finally {
    caseExporting.value = false
  }
}

// DCS-FR-PACM-03: REVIEW runs of the workflow gates wait for a decision.
const gateRuns = ref<PACWorkflowGateRun[]>([])
const gateError = ref<string | null>(null)
const gateJustifications = ref<Record<string, string>>({})
const decidingRun = ref<string | null>(null)

async function loadGateRuns() {
  gateError.value = null
  try {
    gateRuns.value = await pacNonComplianceService.workflowGateRuns()
  } catch (err) {
    claimReportedHttpError(err)
    gateError.value = err instanceof Error ? err.message : 'Workflow-gate reviews could not be loaded.'
  }
}

async function decide(run: PACWorkflowGateRun, decision: 'approve' | 'reject') {
  const justification = gateJustifications.value[run.run_id]?.trim()
  if (!justification) return
  decidingRun.value = run.run_id
  gateError.value = null
  try {
    await pacNonComplianceService.decideWorkflowGateReview(run.run_id, decision, justification)
    await loadGateRuns()
  } catch (err) {
    claimReportedHttpError(err)
    gateError.value = err instanceof Error ? err.message : 'The review decision could not be recorded.'
  } finally {
    decidingRun.value = null
  }
}

onMounted(() => {
  void loadCases()
  if (isComplianceOfficer.value) void loadGateRuns()
})
</script>

<template>
  <div class="mb-4 flex justify-between p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>

  <section class="space-y-6 px-4">
    <div v-if="isComplianceOfficer" class="rounded-box border border-base-content/10 p-4">
      <div class="mb-4 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <label class="form-control w-full md:w-96">
          <span class="label-text mb-1">Filter by contract DID</span>
          <input
            id="input-filter-did"
            v-model="searchTerm"
            data-testid="monitor-search"
            class="input-bordered input rounded-box"
          />
        </label>

        <label class="form-control w-full md:w-96">
          <span class="label-text mb-1">Justification to name denied actors (optional)</span>
          <input
            v-model="monitorJustification"
            data-testid="monitor-justification"
            class="input-bordered input rounded-box"
          />
        </label>

        <button
          type="button"
          class="btn rounded-box btn-primary md:self-end"
          data-testid="run-monitoring-sweep"
          :disabled="sweepLoading"
          @click="runMonitoringSweep"
        >
          <span v-if="sweepLoading" class="loading loading-sm loading-spinner"></span>
          <span v-else>Run monitoring sweep</span>
        </button>
      </div>

      <div v-if="sweepLoading" class="alert rounded-box alert-info" role="status">Running monitoring sweep…</div>
      <div v-else-if="sweepError" class="alert rounded-box alert-error" role="alert">{{ sweepError }}</div>
      <div
        v-else-if="!hasCompletedSweep"
        data-testid="monitor-idle-state"
        class="alert rounded-box alert-info"
        role="status"
      >
        Run a monitoring sweep to check for compliance risks.
      </div>
      <template v-else>
        <p class="mb-3 text-sm opacity-70">Last checked: {{ checkedAt }}</p>
        <div
          v-if="filteredRisks.length === 0"
          data-testid="monitor-empty-state"
          class="alert rounded-box alert-info"
          role="status"
        >
          {{
            hasActiveFilter && risks.length > 0
              ? 'No compliance risks match the current filter.'
              : 'Monitoring sweep completed. No compliance risks were found.'
          }}
        </div>
        <div v-else class="max-w-full overflow-x-auto rounded-box">
          <table class="table table-zebra">
            <thead>
              <tr>
                <th>Contract DID</th>
                <th>Risk Type</th>
                <th>Detail</th>
                <th>Detected At</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="risk in filteredRisks"
                :key="`${risk.did}-${risk.risk_type}-${risk.detected_at}`"
                data-testid="monitor-risk-row"
                :class="isUnderperformance(risk) ? 'bg-warning/10' : ''"
              >
                <td data-testid="monitor-risk-did">
                  <RouterLink
                    :to="{ name: ROUTES.AUDIT.LIST, query: { did: risk.did } }"
                    class="link wrap-anywhere"
                    :aria-label="`Action history of ${risk.did}`"
                  >
                    {{ risk.did }}
                  </RouterLink>
                </td>
                <td data-testid="monitor-risk-type">
                  <span
                    v-if="isUnderperformance(risk)"
                    class="mr-2 badge badge-sm badge-warning"
                    data-testid="monitor-risk-underperformance-badge"
                  >
                    Underperformance
                  </span>
                  {{ risk.risk_type }}
                </td>
                <td data-testid="monitor-risk-detail">{{ risk.detail }}</td>
                <td data-testid="monitor-risk-detected-at">{{ risk.detected_at }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>

    <div v-if="isComplianceOfficer" class="rounded-box border border-base-content/10 p-4" data-testid="gate-reviews">
      <div class="mb-3 flex items-center justify-between gap-2">
        <h3 class="font-bold">Workflow-gate reviews</h3>
        <button type="button" class="btn btn-ghost btn-sm" @click="loadGateRuns">Refresh reviews</button>
      </div>
      <p v-if="gateError" class="mb-2 text-sm text-error">{{ gateError }}</p>
      <p v-if="gateRuns.length === 0" class="text-sm opacity-70">No workflow-gate run waits for a review.</p>
      <ul v-else class="space-y-3">
        <li
          v-for="run in gateRuns"
          :key="run.run_id"
          class="space-y-2 rounded-box border border-base-content/10 p-3"
          :data-testid="`gate-run-${run.run_id}`"
        >
          <div class="text-sm wrap-anywhere">
            <span class="font-medium">{{ run.gate }}</span>
            gate · contract {{ run.contract_did }} · version
            {{ run.contract_version }}
            <span v-if="run.created_at" class="opacity-70">· {{ new Date(run.created_at).toLocaleString() }}</span>
          </div>
          <div class="font-mono text-xs opacity-70">Run {{ run.run_id }}</div>
          <ul v-if="run.findings?.length" class="list-disc pl-5 text-sm">
            <li v-for="finding in run.findings" :key="finding">{{ finding }}</li>
          </ul>
          <div class="flex flex-wrap items-end gap-2">
            <label class="form-control grow">
              <span class="label-text mb-1">Decision justification</span>
              <input
                v-model="gateJustifications[run.run_id]"
                class="input-bordered input input-sm"
                :data-testid="`gate-justification-${run.run_id}`"
              />
            </label>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="decidingRun !== null || !gateJustifications[run.run_id]?.trim()"
              :data-testid="`gate-approve-${run.run_id}`"
              @click="decide(run, 'approve')"
            >
              Approve run
            </button>
            <button
              type="button"
              class="btn btn-outline btn-sm btn-error"
              :disabled="decidingRun !== null || !gateJustifications[run.run_id]?.trim()"
              :data-testid="`gate-reject-${run.run_id}`"
              @click="decide(run, 'reject')"
            >
              Reject run
            </button>
          </div>
        </li>
      </ul>
    </div>

    <div v-if="isComplianceOfficer" class="rounded-box border border-base-content/10 p-4">
      <h3 class="mb-3 font-bold">Submit non-compliance incident report</h3>
      <form data-testid="incident-form" class="grid gap-3 md:grid-cols-2" @submit.prevent="submitIncidentReport">
        <label class="form-control w-full">
          <span class="label-text mb-1">Linked to</span>
          <select v-model="linkedTo" class="select-bordered select rounded-box" data-testid="incident-linked-to">
            <option value="contract">Contract</option>
            <option value="template">Template</option>
          </select>
        </label>

        <label class="form-control w-full">
          <span class="label-text mb-1">{{ linkedTo === 'contract' ? 'Contract DID' : 'Template DID' }}</span>
          <input
            id="input-linked-did"
            v-model="linkedDid"
            data-testid="incident-did"
            class="input-bordered input rounded-box"
            required
          />
        </label>

        <label class="form-control w-full md:col-span-2">
          <span class="label-text mb-1">Risk type</span>
          <input
            id="input-risk-type"
            v-model="riskType"
            data-testid="incident-risk-type"
            class="input-bordered input rounded-box"
            required
          />
        </label>

        <label class="form-control w-full md:col-span-2">
          <span class="label-text mb-1">Finding detail</span>
          <textarea
            id="input-detail"
            v-model="detail"
            data-testid="incident-detail"
            class="textarea-bordered textarea rounded-box"
            required
          ></textarea>
        </label>

        <div class="md:col-span-2">
          <button
            type="submit"
            class="btn rounded-box btn-primary"
            data-testid="incident-submit"
            :disabled="incidentSubmitting || !canSubmitIncident"
          >
            <span v-if="incidentSubmitting" class="loading loading-sm loading-spinner"></span>
            <span v-else>Submit incident report</span>
          </button>
        </div>
      </form>

      <div v-if="incidentError" class="mt-3 alert rounded-box alert-error" role="alert">{{ incidentError }}</div>
      <div
        v-if="incidentSuccess"
        data-testid="incident-success"
        class="mt-3 alert rounded-box alert-success"
        role="status"
      >
        Incident report submitted successfully.
        <span v-if="caseId" data-testid="incident-case-id">Case {{ caseId }}</span>
      </div>
    </div>

    <div class="rounded-box border border-base-content/10 p-4" data-testid="cases">
      <h3 class="mb-3 font-bold">Cases</h3>
      <form class="mb-3 flex flex-wrap items-end gap-2" @submit.prevent="exportCaseFile(caseLookup)">
        <label class="form-control">
          <span class="label-text mb-1">Case ID</span>
          <input v-model="caseLookup" class="input-bordered input input-sm" data-testid="case-lookup" />
        </label>
        <label class="form-control grow">
          <span class="label-text mb-1">Case file justification</span>
          <input
            v-model="caseJustification"
            class="input-bordered input input-sm"
            data-testid="case-justification"
            required
          />
        </label>
        <button
          type="submit"
          class="btn btn-sm"
          :disabled="caseExporting || !caseLookup.trim() || !caseJustification.trim()"
          data-testid="case-export"
        >
          Export case file
        </button>
      </form>
      <p v-if="casesError" class="mb-2 text-sm text-error">{{ casesError }}</p>
      <p v-if="cases.length === 0" class="text-sm opacity-70">No incident cases.</p>
      <div v-else class="max-w-full overflow-x-auto rounded-box">
        <table class="table table-sm">
          <thead>
            <tr>
              <th>Case</th>
              <th>Linked to</th>
              <th>Reported</th>
              <th>Findings</th>
              <th>Last export</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in cases" :key="item.case_id" :data-testid="`case-${item.case_id}`">
              <td class="font-mono text-xs wrap-anywhere">{{ item.case_id }}</td>
              <td class="wrap-anywhere">{{ item.linked_to }} {{ item.did }}</td>
              <td>{{ new Date(item.reported_at).toLocaleString() }} by {{ item.reported_by }}</td>
              <td>{{ item.findings }}</td>
              <td>{{ item.exported_at ? new Date(item.exported_at).toLocaleString() : 'Not exported' }}</td>
              <td>
                <button
                  type="button"
                  class="btn btn-ghost btn-xs"
                  :disabled="caseExporting || !caseJustification.trim()"
                  :data-testid="`case-export-${item.case_id}`"
                  @click="exportCaseFile(item.case_id)"
                >
                  Export
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
