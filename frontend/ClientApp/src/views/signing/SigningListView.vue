<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ROUTES } from '@/router/router'
import {
  type SignatureContract,
  signatureManagementService,
  type SignatureViewResult,
} from '@/services/signature-management-service'
import {
  dssIndicator,
  findingIndicator,
  signatureLevelBadgeClass,
  signatureLevelLabel,
  signatureStatusIndicator,
} from '@/utils/signature-verdict'

// DCS-FR-SM-22: the signer dashboard lists pending (APPROVED), completed
// (SIGNED/ACTIVE), and revoked (REVOKED) contracts. Each row expands into the
// per-signature detail — credential used, signer, timestamps, and validation
// results — read from GET /signature/view. Signing itself is driven step by
// step in the per-contract Secure Contract Viewer.
const contracts = ref<SignatureContract[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// At most one row is expanded; its /signature/view result loads lazily on
// first expansion (the view endpoint runs a full validation pass per call).
const expandedDid = ref<string | null>(null)
const detail = ref<SignatureViewResult | null>(null)
const detailLoading = ref(false)
const detailError = ref<string | null>(null)

const SIGNING_STATES = ['APPROVED', 'SIGNED', 'ACTIVE', 'REVOKED']

interface Indicator {
  label: string
  cls: string
}

function signingStatus(state: string): Indicator {
  switch (state) {
    case 'APPROVED':
      return { label: 'PENDING', cls: 'badge-warning' }
    case 'SIGNED':
    case 'ACTIVE':
      return { label: 'SIGNED', cls: 'badge-success' }
    case 'REVOKED':
      return { label: 'REVOKED', cls: 'badge-error' }
    default:
      return { label: state, cls: 'badge-ghost' }
  }
}

// This dashboard reads a signature record from the signer's side, where the
// in-force state is the signature itself being SIGNED.
function statusIndicator(status: string): Indicator {
  return signatureStatusIndicator(status, 'SIGNED')
}

async function toggleDetails(did: string) {
  if (expandedDid.value === did) {
    expandedDid.value = null
    detail.value = null
    detailError.value = null
    return
  }
  expandedDid.value = did
  detail.value = null
  detailError.value = null
  detailLoading.value = true
  try {
    const result = await signatureManagementService.getSignatureView(did)
    // Ignore a late response after the user switched/closed the expansion.
    if (expandedDid.value === did) detail.value = result
  } catch {
    if (expandedDid.value === did) detailError.value = 'Failed to load signature details.'
  } finally {
    detailLoading.value = false
  }
}

onMounted(async () => {
  loading.value = true
  try {
    const all = await signatureManagementService.retrieveContracts()
    contracts.value = all.filter((c) => SIGNING_STATES.includes(c.state))
  } catch {
    error.value = 'Failed to load contracts for signing.'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">Signing</h2>
  </div>

  <div class="p-4">
    <div v-if="loading" class="text-base-content/70" role="status" aria-live="polite">Loading contracts…</div>
    <div v-else-if="error" class="mb-4 alert alert-error" role="alert" aria-live="assertive">{{ error }}</div>
    <div v-else-if="contracts.length === 0" class="text-base-content/70">
      Nothing awaits your signature. Contracts appear here once they are approved for signing.
    </div>

    <div v-else class="overflow-x-auto">
      <table class="table w-full table-zebra" data-testid="signing-list">
        <thead>
          <tr class="text-base-content/70">
            <th>DID</th>
            <th>Name</th>
            <th>Version</th>
            <th>Updated</th>
            <th>Signing</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-for="contract in contracts" :key="contract.did">
            <tr data-testid="signing-row">
              <td class="max-w-xs truncate font-mono text-xs">{{ contract.did }}</td>
              <td>{{ contract.name ?? '—' }}</td>
              <td>{{ contract.contract_version ?? 1 }}</td>
              <td>{{ new Date(contract.updated_at).toLocaleDateString() }}</td>
              <td>
                <span class="badge badge-sm" :class="signingStatus(contract.state).cls" data-testid="signing-status">
                  {{ signingStatus(contract.state).label }}
                </span>
              </td>
              <td class="text-right whitespace-nowrap">
                <button
                  class="btn mr-2 btn-ghost btn-sm"
                  data-testid="signing-details-toggle"
                  @click="toggleDetails(contract.did)"
                >
                  {{ expandedDid === contract.did ? 'Hide signatures' : 'Signatures' }}
                </button>
                <RouterLink
                  class="btn btn-sm btn-primary"
                  :to="{ name: ROUTES.SIGNING.VIEWER, params: { did: contract.did } }"
                >
                  {{ contract.state === 'APPROVED' ? 'Open & sign' : 'Open' }}
                </RouterLink>
              </td>
            </tr>
            <tr v-if="expandedDid === contract.did">
              <td colspan="6" class="bg-base-200/50" data-testid="signing-signatures">
                <div v-if="detailLoading" class="text-base-content/70" role="status" aria-live="polite">
                  Loading signature details…
                </div>
                <div v-else-if="detailError" class="alert alert-error" role="alert">{{ detailError }}</div>
                <template v-else-if="detail">
                  <div v-if="detail.signatures.length === 0" class="text-base-content/70">
                    No signature has been applied yet.
                  </div>
                  <div
                    v-for="sig in detail.signatures"
                    :key="sig.signer_did + (sig.field_name ?? '')"
                    class="flex flex-wrap items-center gap-x-4 gap-y-1 py-1"
                    data-testid="signature-entry"
                  >
                    <span
                      class="badge badge-sm"
                      :class="signatureLevelBadgeClass(signatureLevelLabel(sig.credential_type))"
                      data-testid="signature-credential"
                    >
                      {{ signatureLevelLabel(sig.credential_type) }}
                    </span>
                    <span
                      class="badge badge-sm"
                      :class="statusIndicator(sig.status).cls"
                      data-testid="signature-status"
                    >
                      {{ statusIndicator(sig.status).label }}
                    </span>
                    <span class="max-w-xs truncate font-mono text-xs" data-testid="signature-signer">
                      {{ sig.signer_did }}
                    </span>
                    <span class="text-xs" data-testid="signature-timestamp">
                      Signed at: {{ sig.signed_at ?? '—' }}
                      <template v-if="sig.revoked_at">· Revoked at: {{ sig.revoked_at }}</template>
                    </span>
                  </div>
                  <div class="mt-2 border-t border-base-content/10 pt-2" data-testid="signing-validation">
                    <span class="mr-2 text-xs font-semibold">Validation:</span>
                    <span
                      v-if="detail.dss"
                      class="mr-2 badge badge-sm"
                      :class="dssIndicator(detail.dss.indication).cls"
                    >
                      DSS {{ dssIndicator(detail.dss.indication).label }}
                    </span>
                    <div v-if="detail.integrity_findings.length" class="mt-1 space-y-0.5">
                      <div
                        v-for="finding in detail.integrity_findings"
                        :key="finding"
                        class="flex items-center gap-2 text-xs"
                      >
                        <span class="badge badge-sm" :class="findingIndicator(finding).cls">
                          {{ findingIndicator(finding).label }}
                        </span>
                        <span>{{ finding }}</span>
                      </div>
                    </div>
                    <span v-else-if="!detail.dss" class="text-xs text-base-content/70">
                      No validation result available yet.
                    </span>
                  </div>
                </template>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>
