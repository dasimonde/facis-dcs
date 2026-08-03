<script setup lang="ts">
import { useConfirmDialog } from '@vueuse/core'
import { useQRCode } from '@vueuse/integrations/useQRCode'
import { computed, ref, useTemplateRef, watch } from 'vue'
import { type CeremonyStatus, signatureManagementService } from '@/services/signature-management-service'

interface CeremonyRequest {
  contractDid: string
  fieldName: string
}

export interface CeremonyResult {
  signerDid: string
  // The EXACT ceremony this result came from — callers MUST pass this to
  // BOTH prepareSignature and submitSignature so they resolve the same
  // ceremony prepare pinned bytes on (ADR-20), rather than "the signer's
  // most recent verified ceremony", which is ambiguous once more than one
  // ceremony has been verified for the same signer/field (e.g. a retried
  // ceremony after "Start a new ceremony").
  ceremonyId: string
}

const POLL_INTERVAL_MS = 2500
const dialogTitleId = 'signing-ceremony-title'
const dialogDescriptionId = 'signing-ceremony-description'

const ceremonyModal = useTemplateRef('ceremony-modal')
const request = ref<CeremonyRequest>({ contractDid: '', fieldName: '' })

const phase = ref<'starting' | CeremonyStatus>('starting')
const walletUri = ref('')
const errorMessage = ref('')
const qrCodeDataUrl = useQRCode(computed(() => walletUri.value || ''))
const copyHint = ref('')

let ceremonyId = ''
let pollTimer: ReturnType<typeof setInterval> | undefined

const { isRevealed, reveal, confirm, cancel, onReveal } = useConfirmDialog<
  CeremonyRequest,
  CeremonyResult | undefined
>()

onReveal((data) => {
  if (data) request.value = data
  void startCeremony()
})

watch(isRevealed, (value) => {
  if (value) {
    ceremonyModal.value?.showModal()
    focusDialog()
  } else {
    stopPolling()
    ceremonyModal.value?.close()
  }
})

function focusDialog() {
  window.requestAnimationFrame(() => {
    ceremonyModal.value?.focus()
  })
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
}

async function startCeremony() {
  stopPolling()
  phase.value = 'starting'
  errorMessage.value = ''
  walletUri.value = ''
  try {
    const started = await signatureManagementService.startCeremony(request.value.contractDid, request.value.fieldName)
    ceremonyId = started.ceremony_id
    walletUri.value = started.wallet_uri
    copyHint.value = ''
    phase.value = 'pending'
    pollTimer = setInterval(() => void pollStatus(), POLL_INTERVAL_MS)
  } catch (e: unknown) {
    phase.value = 'failed'
    errorMessage.value = e instanceof Error ? e.message : 'Could not start the signing ceremony.'
  }
}

async function pollStatus() {
  if (!ceremonyId) return
  try {
    const status = await signatureManagementService.getCeremonyStatus(ceremonyId)
    phase.value = status.status
    if (status.status === 'verified') {
      stopPolling()
      confirm({ signerDid: status.signer_did ?? '', ceremonyId })
    } else if (status.status === 'expired' || status.status === 'failed') {
      stopPolling()
    }
  } catch {
    // Transient poll failures are ignored; the interval retries on the next tick.
  }
}

async function copyWalletUri() {
  if (!walletUri.value) return
  await navigator.clipboard.writeText(walletUri.value)
  copyHint.value = 'Link copied.'
}

function retry() {
  void startCeremony()
}

function onCancel() {
  stopPolling()
  cancel()
}

interface DialogExpose {
  reveal: (data: CeremonyRequest) => Promise<{ isCanceled: boolean; data?: CeremonyResult }>
}

defineExpose<DialogExpose>({ reveal })
</script>

<template>
  <Teleport to="body">
    <dialog
      ref="ceremony-modal"
      class="modal modal-bottom transition-none sm:modal-middle"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="dialogTitleId"
      :aria-describedby="dialogDescriptionId"
      @close="onCancel"
    >
      <div class="modal-box flex w-full max-w-md flex-col items-center gap-4 text-center">
        <h3 :id="dialogTitleId" class="text-lg font-bold">Sign with your wallet</h3>

        <div v-if="phase === 'starting'" class="flex flex-col items-center gap-3 py-4" role="status" aria-live="polite">
          <span class="loading loading-lg loading-spinner" aria-hidden="true" />
          <p :id="dialogDescriptionId" class="text-sm opacity-70">Starting signing ceremony…</p>
        </div>

        <div v-else-if="phase === 'pending'" class="flex flex-col items-center gap-3">
          <p :id="dialogDescriptionId" class="text-sm opacity-80">
            Scan the QR code with your wallet to present your PID and Power of Attorney, then sign.
          </p>
          <figure class="rounded-box bg-white p-3">
            <img v-if="qrCodeDataUrl" :src="qrCodeDataUrl" alt="Signing ceremony QR code" class="mx-auto h-48 w-48" />
          </figure>
          <button type="button" class="btn btn-sm btn-primary" @click="copyWalletUri">Copy link</button>
          <p v-if="copyHint" class="text-sm text-warning" role="status" aria-live="polite">{{ copyHint }}</p>
          <p class="text-xs opacity-70" role="status" aria-live="polite">Waiting for the wallet presentation…</p>
        </div>

        <div
          v-else-if="phase === 'expired' || phase === 'failed'"
          class="flex flex-col items-center gap-3 py-2"
          role="alert"
        >
          <p :id="dialogDescriptionId" class="text-sm text-error">
            {{ phase === 'expired' ? 'The signing ceremony expired.' : errorMessage || 'The signing ceremony failed.' }}
          </p>
          <button type="button" class="btn btn-sm btn-primary" @click="retry">Start a new ceremony</button>
        </div>

        <div class="modal-action mt-2">
          <button type="button" class="btn btn-outline btn-sm" @click="onCancel">Cancel</button>
        </div>
      </div>
      <div class="modal-backdrop" @click="onCancel"></div>
    </dialog>
  </Teleport>
</template>
