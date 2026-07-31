<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import type { MachineCredential } from '@/models/responses/contract-response'

/**
 * Shows an issued machine credential (ADR-27). The secret is in the response
 * that opened this dialog and in no other: Hydra keeps only a hash, so closing
 * without copying means rotating, not recovering. The wording says so rather
 * than leaving the operator to discover it.
 */

const props = defineProps<{
  credential: MachineCredential
  title: string
  returnFocusTo?: HTMLElement | null
}>()
const emit = defineEmits<{ close: [] }>()

const copied = ref(false)
const dialog = ref<HTMLDialogElement | null>(null)
const clientId = ref<HTMLInputElement | null>(null)
const returnFocusTarget =
  props.returnFocusTo ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)

onMounted(async () => {
  dialog.value?.showModal()
  await nextTick()
  clientId.value?.focus()
})

const closeDialog = () => {
  dialog.value?.close()
}

const keepFocusInside = (event: KeyboardEvent) => {
  if (event.key !== 'Tab' || !dialog.value) return

  const focusable = Array.from(
    dialog.value.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])'),
  )
  if (focusable.length === 0) return

  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first?.focus()
  }
}

const handleClose = () => {
  emit('close')
  window.requestAnimationFrame(() => returnFocusTarget?.focus())
}

const copy = async (value: string) => {
  await navigator.clipboard.writeText(value)
  copied.value = true
}
</script>

<template>
  <dialog
    ref="dialog"
    class="modal modal-bottom transition-none sm:modal-middle"
    data-testid="credential-dialog"
    aria-labelledby="credential-dialog-title"
    aria-describedby="credential-dialog-description"
    @close="handleClose"
    @keydown="keepFocusInside"
  >
    <div class="modal-box max-w-2xl">
      <h3 id="credential-dialog-title" class="text-lg font-semibold">{{ title }}</h3>

      <div class="mt-4 alert alert-warning">
        <span id="credential-dialog-description" data-testid="credential-once-warning">
          This secret is shown once. It is not stored and cannot be retrieved — if it is lost, issue a new one, which
          stops this one working.
        </span>
      </div>

      <div class="mt-4 flex flex-col gap-3">
        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Client ID</span>
          <input
            ref="clientId"
            :value="credential.client_id"
            data-testid="credential-client-id"
            readonly
            class="input-bordered input w-full min-w-0 font-mono text-sm"
          />
        </label>

        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Client secret</span>
          <div class="flex min-w-0 flex-col gap-2 sm:flex-row sm:gap-0">
            <input
              :value="credential.client_secret"
              data-testid="credential-secret"
              readonly
              class="input-bordered input w-full min-w-0 font-mono text-sm sm:rounded-r-none"
            />
            <button
              type="button"
              class="btn shrink-0 sm:rounded-l-none"
              data-testid="credential-copy"
              @click="copy(credential.client_secret)"
            >
              {{ copied ? 'Copied' : 'Copy' }}
            </button>
          </div>
        </label>

        <label v-if="credential.token_url" class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Token endpoint</span>
          <input
            :value="credential.token_url"
            data-testid="credential-token-url"
            readonly
            class="input-bordered input w-full min-w-0 font-mono text-sm"
          />
          <span class="label-text-alt opacity-70">Present the credential here with grant_type=client_credentials.</span>
        </label>
      </div>

      <div class="modal-action">
        <button type="button" class="btn btn-primary" data-testid="credential-done" @click="closeDialog">
          I have copied it
        </button>
      </div>
    </div>
  </dialog>
</template>
