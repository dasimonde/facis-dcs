<script setup lang="ts">
import {
  signatureManagementService,
  type SignatureComplianceResult,
  type SignatureContract,
  type SignatureEnvelope,
  type SignatureValidateResult,
  type SignatureVerifyResult,
} from '@/services/signature-management-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { onMounted, ref } from 'vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'

const authStore = useAuthStore()

const contracts = ref<SignatureContract[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const { isSigner } = useContractPermissions()

// Per-contract state: signing in progress, result envelope, verify result.
const signing = ref<Record<string, boolean>>({})
const envelopes = ref<Record<string, SignatureEnvelope | undefined>>({})
const verifyResults = ref<Record<string, SignatureVerifyResult | undefined>>({})
const validateResults = ref<Record<string, SignatureValidateResult | undefined>>({})
const complianceResults = ref<Record<string, SignatureComplianceResult | undefined>>({})
const pdfVerifyResults = ref<
  Record<
    string,
    | {
        lifecycle_status?: string
        status_list_status?: string
      }
    | undefined
  >
>({})

// DCS-OR-C2PA-006: derive human-readable C2PA lifecycle banner label and CSS class
// from the contract state. Returns one of: Active, Draft, Suspended, Terminated,
// Replaced, Expired, or the raw state as fallback.
interface C2PAStatus {
  label: string
  cls: string
}
function c2paStatus(contract: SignatureContract): C2PAStatus {
  const pdfVerify = pdfVerifyResults.value[contract.did]
  const lifecycle = (pdfVerify?.lifecycle_status ?? '').toLowerCase()
  const statusList = (pdfVerify?.status_list_status ?? '').toLowerCase()
  if (statusList === 'revoked') {
    return { label: 'Suspended', cls: 'badge-warning' }
  }

  const lifecycleMap: Record<string, C2PAStatus> = {
    active: { label: 'Active', cls: 'badge-success' },
    draft: { label: 'Draft', cls: 'badge-ghost' },
    suspended: { label: 'Suspended', cls: 'badge-warning' },
    terminated: { label: 'Terminated', cls: 'badge-error' },
    replaced: { label: 'Replaced', cls: 'badge-neutral' },
    expired: { label: 'Expired', cls: 'badge-neutral' },
  }
  if (lifecycleMap[lifecycle]) {
    return lifecycleMap[lifecycle]
  }

  const state = (contract.state ?? '').toLowerCase()
  const map: Record<string, C2PAStatus> = {
    active: { label: 'Active', cls: 'badge-success' },
    approved: { label: 'Active', cls: 'badge-success' },
    draft: { label: 'Draft', cls: 'badge-ghost' },
    suspended: { label: 'Suspended', cls: 'badge-warning' },
    terminated: { label: 'Terminated', cls: 'badge-error' },
    replaced: { label: 'Replaced', cls: 'badge-neutral' },
    expired: { label: 'Expired', cls: 'badge-neutral' },
    amended: { label: 'Active', cls: 'badge-success' },
  }
  return map[state] ?? { label: contract.state ?? 'Unknown', cls: 'badge-ghost' }
}

onMounted(async () => {
  loading.value = true
  try {
    contracts.value = await signatureManagementService.retrieveContracts()
  } catch {
    error.value = 'Failed to load contracts for signing.'
  } finally {
    loading.value = false
  }
})

async function sign(contract: SignatureContract) {
  const issuer = authStore.user?.issuer ?? 'unknown'
  signing.value[contract.did] = true
  try {
    const env = await signatureManagementService.applySignature(contract.did, issuer)
    envelopes.value[contract.did] = env
  } catch (e: unknown) {
    error.value = `Failed to sign contract ${contract.did}: ${e instanceof Error ? e.message : String(e)}`
  } finally {
    signing.value[contract.did] = false
  }
}

async function verify(contract: SignatureContract) {
  try {
    verifyResults.value[contract.did] = await signatureManagementService.verifySignature(contract.did)
    pdfVerifyResults.value[contract.did] = await contractWorkflowService.verifyPdf(contract.did)
  } catch (e: unknown) {
    error.value = `Failed to verify contract ${contract.did}: ${e instanceof Error ? e.message : String(e)}`
  }
}

async function validate(contract: SignatureContract) {
  try {
    validateResults.value[contract.did] = await signatureManagementService.validateSignature(contract.did)
  } catch (e: unknown) {
    error.value = `Failed to validate contract ${contract.did}: ${e instanceof Error ? e.message : String(e)}`
  }
}

async function compliance(contract: SignatureContract) {
  try {
    complianceResults.value[contract.did] = await signatureManagementService.complianceCheck(contract.did)
  } catch (e: unknown) {
    error.value = `Failed to run compliance check for ${contract.did}: ${e instanceof Error ? e.message : String(e)}`
  }
}
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">Signing Dashboard</h2>
  </div>

  <div class="p-4">
    <div v-if="loading" class="text-base-content/60">Loading approved contracts…</div>
    <div v-else-if="error" class="mb-4 alert alert-error">{{ error }}</div>
    <div v-else-if="contracts.length === 0" class="text-base-content/60">
      No approved contracts available for signing.
    </div>

    <div v-else class="overflow-x-auto">
      <table class="table w-full table-zebra">
        <thead>
          <tr>
            <th>DID</th>
            <th>Name</th>
            <th>Version</th>
            <th>Updated</th>
            <th>C2PA Status</th>
            <th>Signature</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="contract in contracts" :key="contract.did">
            <td class="max-w-xs truncate font-mono text-xs">{{ contract.did }}</td>
            <td>{{ contract.name ?? '—' }}</td>
            <td>{{ contract.contract_version ?? 1 }}</td>
            <td>{{ new Date(contract.updated_at).toLocaleDateString() }}</td>
            <!-- DCS-OR-C2PA-006: C2PA lifecycle status banner -->
            <td>
              <span :class="['badge', 'badge-sm', c2paStatus(contract).cls]">
                {{ c2paStatus(contract).label }}
              </span>
            </td>
            <td>
              <span
                v-if="envelopes[contract.did]"
                :class="['badge', envelopes[contract.did]?.status === 'SIGNED' ? 'badge-success' : 'badge-warning']"
              >
                {{ envelopes[contract.did]?.status }}
              </span>
              <span v-else class="badge badge-ghost">UNSIGNED</span>

              <div
                v-if="verifyResults[contract.did]"
                class="mt-1 text-xs"
                :class="verifyResults[contract.did]?.match ? 'text-success' : 'text-error'"
              >
                MR/HR: {{ verifyResults[contract.did]?.match ? 'match ✓' : 'mismatch ✗' }} ({{
                  verifyResults[contract.did]?.sig_count
                }}
                sig(s))
              </div>
              <div v-if="verifyResults[contract.did]?.findings?.length" class="mt-1 text-xs">
                Verify: {{ verifyResults[contract.did]?.findings?.[0] }}
              </div>

              <div v-if="validateResults[contract.did]?.findings?.length" class="mt-1 text-xs">
                Validation: {{ validateResults[contract.did]?.findings?.[0] }}
              </div>
              <div v-if="complianceResults[contract.did]?.findings?.length" class="mt-1 text-xs">
                Compliance: {{ complianceResults[contract.did]?.findings?.[0] }}
              </div>
            </td>
            <td class="flex gap-2">
              <button
                class="btn btn-sm btn-primary"
                :disabled="envelopes[contract.did]?.status === 'SIGNED' || !isSigner || signing[contract.did]"
                @click="sign(contract)"
              >
                <span v-if="signing[contract.did]" class="loading loading-xs loading-spinner" />
                Sign
              </button>
              <button class="btn btn-outline btn-sm" :disabled="!isSigner" @click="verify(contract)">Verify</button>
              <button class="btn btn-outline btn-sm" :disabled="!isSigner" @click="validate(contract)">Validate</button>
              <button class="btn btn-outline btn-sm" :disabled="!isSigner" @click="compliance(contract)">
                Compliance
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
