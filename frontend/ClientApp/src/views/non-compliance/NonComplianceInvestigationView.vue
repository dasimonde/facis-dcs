<script setup lang="ts">
import { computed, ref } from 'vue'
import { pacNonComplianceService } from '@/services/pac-non-compliance-service'
import { claimReportedHttpError } from '@/utils/report-action-error'
import type { PACComplianceRisk } from '@/models/responses/pac-non-compliance-response'

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

const runMonitoringSweep = async () => {
  sweepLoading.value = true
  sweepError.value = null
  try {
    const response = await pacNonComplianceService.monitor()
    checkedAt.value = response.checked_at
    risks.value = response.risks
  } catch (err) {
    claimReportedHttpError(err)
    sweepError.value = err instanceof Error ? err.message : 'Monitoring sweep could not be executed.'
  } finally {
    sweepLoading.value = false
  }
}

const contractDid = ref('')
// A contract in force that no longer meets its own agreed boundaries is the
// one risk here that is about the contract UNDERPERFORMING rather than about a
// process step being missed, so it is called out visually (DCS-FR-CWE-31).
const isUnderperformance = (risk: PACComplianceRisk) => risk.risk_type === 'CONTRACT_UNDERPERFORMANCE'

const riskType = ref('')
const detail = ref('')
const incidentSubmitting = ref(false)
const incidentError = ref<string | null>(null)
const incidentSuccess = ref(false)

const canSubmitIncident = computed(
  () => contractDid.value.trim() !== '' && riskType.value.trim() !== '' && detail.value.trim() !== '',
)

const submitIncidentReport = async () => {
  incidentSubmitting.value = true
  incidentError.value = null
  incidentSuccess.value = false
  try {
    await pacNonComplianceService.reportIncident({
      contract_did: contractDid.value.trim(),
      findings: [{ risk_type: riskType.value.trim(), detail: detail.value.trim() }],
    })
    incidentSuccess.value = true
  } catch (err) {
    claimReportedHttpError(err)
    incidentError.value = err instanceof Error ? err.message : 'Incident report could not be submitted.'
  } finally {
    incidentSubmitting.value = false
  }
}
</script>

<template>
  <div class="mb-4 flex justify-between p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>

  <section class="space-y-6 px-4">
    <div class="rounded-box border border-base-content/10 p-4">
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
                <td data-testid="monitor-risk-did">{{ risk.did }}</td>
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

    <div class="rounded-box border border-base-content/10 p-4">
      <h3 class="mb-3 font-bold">Submit non-compliance incident report</h3>
      <form data-testid="incident-form" class="grid gap-3 md:grid-cols-2" @submit.prevent="submitIncidentReport">
        <label class="form-control w-full">
          <span class="label-text mb-1">Contract DID</span>
          <input
            id="input-contract-did"
            v-model="contractDid"
            data-testid="incident-contract-did"
            class="input-bordered input rounded-box"
            required
          />
        </label>

        <label class="form-control w-full">
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
      </div>
    </div>
  </section>
</template>
