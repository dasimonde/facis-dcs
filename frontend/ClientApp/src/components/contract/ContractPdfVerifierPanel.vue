<script setup lang="ts">
import { computed, ref } from 'vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import type { ContractPdfVerifyResult } from '@/models/responses/contract-response'
import type { UserRole } from '@/types/user-role'

// The C2PA verifier (DCS-OR-C2PA-006, -008): the four named checks and the
// lifecycle banner, for the stored contract PDF or a copy someone holds.
const props = defineProps<{ did: string }>()

/** The roles GET and POST /pdf/verify/contract/{did} accept. */
const verifierRoles: readonly UserRole[] = [
  'CONTRACT_MANAGER',
  'CONTRACT_REVIEWER',
  'CONTRACT_APPROVER',
  'CONTRACT_OBSERVER',
  'AUDITOR',
]

const authStore = useAuthStore()
const canVerify = computed(() => verifierRoles.some((role) => authStore.user?.roles?.includes(role)))

const result = ref<ContractPdfVerifyResult | null>(null)
const subject = ref('')
const verifying = ref(false)
const error = ref('')

async function run(label: string, verify: () => Promise<ContractPdfVerifyResult>) {
  verifying.value = true
  error.value = ''
  result.value = null
  subject.value = label
  try {
    result.value = await verify()
  } catch {
    error.value = 'The verification could not be completed.'
  } finally {
    verifying.value = false
  }
}

const verifyStored = () => run('Stored contract PDF', () => contractWorkflowService.verifyPdf(props.did))

async function verifyCopy(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  await run(`Copy ${file.name}`, () => contractWorkflowService.verifyPdfCopy(props.did, file))
}

/** The SRS banner vocabulary (DCS-OR-C2PA-006). */
const banners: Record<string, { label: string; tone: string }> = {
  active: { label: 'Active', tone: 'alert-success' },
  suspended: { label: 'Suspended', tone: 'alert-warning' },
  terminated: { label: 'Terminated', tone: 'alert-error' },
  replaced: { label: 'Replaced', tone: 'alert-warning' },
  expired: { label: 'Expired', tone: 'alert-warning' },
  draft: { label: 'Draft', tone: 'alert-info' },
}
const banner = computed(() => {
  const status = result.value?.lifecycle_status ?? ''
  return banners[status] ?? { label: status || 'Unknown', tone: 'alert-info' }
})

const statusLabel: Record<string, string> = {
  valid: 'Valid',
  invalid: 'Invalid',
  indeterminate: 'Indeterminate',
  not_available: 'Not available',
  passed: 'Passed',
  failed: 'Failed',
}
const badge = (status: string) =>
  ({ valid: 'badge-success', passed: 'badge-success', invalid: 'badge-error', failed: 'badge-error' })[status] ??
  'badge-ghost'

const checks = computed(() => {
  const r = result.value
  if (!r) return []
  const manifest = r.c2pa_manifest_found ? r.c2pa_signature_status : 'not_available'
  return [
    {
      name: 'PDF signature',
      status: r.pdf_signature_status,
      detail: [r.pdf_signature_source ? `Checked by ${r.pdf_signature_source}` : '', r.pdf_signature_detail ?? '']
        .filter(Boolean)
        .join('. '),
    },
    {
      name: 'C2PA manifest',
      status: manifest,
      detail: [
        r.c2pa_manifest_source ? `${r.c2pa_manifest_source} manifest` : '',
        r.c2pa_signer_status ? `Signers: ${statusLabel[r.c2pa_signer_status] ?? r.c2pa_signer_status}` : '',
        r.c2pa_signer_detail ?? '',
      ]
        .filter(Boolean)
        .join('. '),
    },
    { name: 'VC signature', status: r.vc_proof_status, detail: '' },
    {
      name: 'Status list',
      status: r.status_list_check,
      detail: [r.status_list_status ?? '', r.status_list_error ?? ''].filter(Boolean).join('. '),
    },
  ]
})
</script>

<template>
  <div v-if="canVerify" class="card border border-base-300 bg-base-100 shadow-sm" data-testid="pdf-verifier">
    <div class="card-body gap-3">
      <h2 class="card-title text-sm">Verify PDF</h2>
      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-outline btn-sm" :disabled="verifying" @click="verifyStored">
          Verify stored PDF
        </button>
        <label class="btn btn-outline btn-sm" :class="{ 'btn-disabled': verifying }">
          Verify a copy
          <input
            type="file"
            accept="application/pdf"
            class="hidden"
            aria-label="PDF copy to verify"
            :disabled="verifying"
            @change="verifyCopy"
          />
        </label>
        <span v-if="verifying" class="loading loading-sm loading-spinner" role="status" aria-label="Verifying"></span>
      </div>
      <p v-if="error" class="alert alert-error" role="alert">{{ error }}</p>
      <div v-if="result" class="flex flex-col gap-2" data-testid="pdf-verifier-result">
        <p class="text-xs text-base-content/70">{{ subject }}</p>
        <div class="alert" :class="banner.tone" role="status" data-testid="pdf-verifier-banner">
          <span class="font-bold">{{ banner.label }}</span>
        </div>
        <p
          :class="result.match ? 'text-success' : 'text-error'"
          class="text-sm font-medium"
          data-testid="pdf-verifier-match"
        >
          {{
            result.match ? 'The PDF matches the contract.' : `Mismatch: ${result.discrepancy ?? 'verification failed'}`
          }}
        </p>
        <ul class="flex flex-col gap-1">
          <li
            v-for="check in checks"
            :key="check.name"
            class="flex flex-wrap items-center gap-2 text-sm"
            data-testid="pdf-verifier-check"
          >
            <span class="w-32 font-medium">{{ check.name }}</span>
            <span class="badge badge-sm" :class="badge(check.status)">
              {{ statusLabel[check.status] ?? check.status }}
            </span>
            <span v-if="check.detail" class="text-xs text-base-content/60">{{ check.detail }}</span>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>
