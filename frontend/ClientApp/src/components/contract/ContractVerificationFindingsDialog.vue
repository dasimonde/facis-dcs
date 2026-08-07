<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { VerificationResult } from '@contract-workflow-engine/composables/useSemanticValueVerification'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  disabled?: boolean
  verify: () => VerificationResult
  submit: (comment: string) => Promise<void>
}>()

const trigger = ref<HTMLButtonElement | null>(null)
const dialog = ref<HTMLDialogElement | null>(null)
const findings = ref<VerificationResult['errors']>([])
const verificationSucceeded = ref(false)
const verificationError = ref('')
const submissionError = ref('')
const isVerifying = ref(false)
const isSubmitting = ref(false)
const comment = ref('')

const isBusy = computed(() => isVerifying.value || isSubmitting.value)
const canConfirm = computed(() => verificationSucceeded.value && findings.value.length === 0)

function reset() {
  findings.value = []
  verificationSucceeded.value = false
  verificationError.value = ''
  submissionError.value = ''
  isVerifying.value = false
  isSubmitting.value = false
  comment.value = ''
}

function focusFirstAction() {
  window.requestAnimationFrame(() => {
    dialog.value?.querySelector<HTMLElement>('button:not([disabled]), textarea:not([disabled])')?.focus()
  })
}

async function runVerification() {
  if (isBusy.value) return
  verificationError.value = ''
  submissionError.value = ''
  findings.value = []
  verificationSucceeded.value = false
  isVerifying.value = true
  await nextTick()
  try {
    const result = props.verify()
    findings.value = result.errors
    verificationSucceeded.value = true
  } catch (error) {
    console.error('Local semantic precheck failed', error)
    verificationError.value =
      'Local semantic precheck failed. Fix the contract rules or retry the local verification before approving.'
  } finally {
    isVerifying.value = false
    await nextTick()
    focusFirstAction()
  }
}

async function openDialog() {
  if (props.disabled || isBusy.value || dialog.value?.open) return
  reset()
  dialog.value?.showModal()
  await nextTick()
  dialog.value?.focus()
  await runVerification()
}

function closeDialog() {
  if (isBusy.value) return
  dialog.value?.close()
}

function restoreTriggerFocus() {
  reset()
  window.requestAnimationFrame(() => trigger.value?.focus())
}

async function submitApproval() {
  if (!canConfirm.value || isBusy.value) return
  submissionError.value = ''
  isSubmitting.value = true
  try {
    await props.submit(comment.value.trim())
    dialog.value?.close()
  } catch (error) {
    console.error('Contract review submission failed', error)
    submissionError.value =
      'Submission failed. The local semantic precheck passed, but forwarding the review decision did not complete.'
  } finally {
    isSubmitting.value = false
    await nextTick()
    focusFirstAction()
  }
}

function handleCancel(event: Event) {
  if (isBusy.value) event.preventDefault()
}
</script>

<template>
  <button ref="trigger" type="button" v-bind="$attrs" :disabled="disabled || isBusy" @click="openDialog">
    <span v-if="isBusy" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
    Approve
  </button>

  <Teleport to="body">
    <dialog
      ref="dialog"
      class="modal modal-bottom transition-none sm:modal-middle"
      role="dialog"
      aria-modal="true"
      aria-labelledby="contract-local-precheck-title"
      aria-describedby="contract-local-precheck-description"
      @cancel="handleCancel"
      @close="restoreTriggerFocus"
    >
      <div class="modal-box flex max-h-[85vh] w-full max-w-lg flex-col">
        <h3 id="contract-local-precheck-title" class="text-lg font-bold">Local semantic precheck</h3>
        <p id="contract-local-precheck-description" class="mt-2 text-sm text-base-content/70">
          This local semantic precheck validates the values stored in the contract. It does not replace a full policy or
          SHACL check.
        </p>

        <div v-if="isVerifying" class="my-5 flex items-center gap-3" role="status">
          <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
          Running local semantic precheck…
        </div>

        <div v-else-if="verificationError" class="my-5 alert alert-error" role="alert">
          <span>{{ verificationError }}</span>
          <button type="button" class="btn btn-sm" @click="runVerification">Retry verification</button>
        </div>

        <div v-else-if="verificationSucceeded && findings.length > 0" class="my-5">
          <div class="alert alert-warning">
            Local semantic precheck returned {{ findings.length }} {{ findings.length === 1 ? 'finding' : 'findings' }}.
            Resolve every finding before approving.
          </div>
          <ul class="mt-3 max-h-72 space-y-2 overflow-y-auto" aria-label="Local semantic findings">
            <li
              v-for="finding in findings"
              :key="`${finding.blockId}:${finding.conditionId}:${finding.parameterName}:${finding.message}`"
              class="rounded-box border border-base-300 bg-base-100 p-3 text-sm break-words"
            >
              {{ finding.message }}
            </li>
          </ul>
        </div>

        <div v-else-if="verificationSucceeded" class="my-5 alert alert-success" role="status">
          Local semantic precheck completed with no findings.
        </div>

        <div v-if="submissionError" class="my-5 alert alert-error" role="alert">
          <span>{{ submissionError }}</span>
          <button type="button" class="btn btn-sm" :disabled="isSubmitting" @click="submitApproval">
            Retry submission
          </button>
        </div>

        <label v-if="canConfirm" class="form-control">
          <span class="label-text mb-1">Comment (optional)</span>
          <textarea
            v-model="comment"
            class="textarea-bordered textarea min-h-20 w-full resize-y"
            placeholder="Add context for the approver"
            :disabled="isSubmitting"
          ></textarea>
        </label>

        <div class="modal-action mt-2">
          <button
            v-if="canConfirm && !submissionError"
            type="button"
            class="btn btn-primary"
            :disabled="isSubmitting"
            @click="submitApproval"
          >
            <span v-if="isSubmitting" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
            Confirm approval
          </button>
          <button type="button" class="btn btn-outline" :disabled="isBusy" @click="closeDialog">Cancel</button>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>
