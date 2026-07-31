<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { contractTemplateService } from '@/services/contract-template-service'

defineOptions({ inheritAttrs: false })

const emit = defineEmits<{
  confirm: [comment: string]
}>()

const draftStore = useDcsDraftStore()
const trigger = ref<HTMLButtonElement | null>(null)
const findingsModal = ref<HTMLDialogElement | null>(null)
const findings = ref<string[]>([])
const error = ref('')
const isVerifying = ref(false)
const verificationSucceeded = ref(false)
const isConfirming = ref(false)
const comment = ref('')

function resetResult() {
  error.value = ''
  findings.value = []
  verificationSucceeded.value = false
  isConfirming.value = false
  comment.value = ''
}

async function verifyTemplate() {
  const did = draftStore.did
  if (!did || isVerifying.value) return

  error.value = ''
  findings.value = []
  verificationSucceeded.value = false
  isVerifying.value = true
  try {
    const verificationResult = await contractTemplateService.verify({ did })
    findings.value = verificationResult.findings
    verificationSucceeded.value = true
  } catch (err) {
    console.error('Template verification failed', err)
    error.value = 'Verification could not be completed. Retry the verification before approving.'
  } finally {
    isVerifying.value = false
    await nextTick()
    focusFirstAction()
  }
}

async function openModal() {
  if (isVerifying.value) return
  resetResult()
  findingsModal.value?.showModal()
  await nextTick()
  findingsModal.value?.focus()
  await verifyTemplate()
}

function focusFirstAction() {
  window.requestAnimationFrame(() => {
    findingsModal.value?.querySelector<HTMLElement>('button:not([disabled])')?.focus()
  })
}

function closeModal() {
  findingsModal.value?.close()
}

function restoreTriggerFocus() {
  resetResult()
  window.requestAnimationFrame(() => trigger.value?.focus())
}

function confirmApproval() {
  if (!verificationSucceeded.value || findings.value.length > 0 || isConfirming.value) return
  isConfirming.value = true
  emit('confirm', comment.value.trim())
  closeModal()
}
</script>

<template>
  <button ref="trigger" type="button" v-bind="$attrs" @click="openModal">
    <span v-if="isVerifying" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
    Approve
  </button>

  <Teleport to="body">
    <dialog
      ref="findingsModal"
      class="modal modal-bottom transition-none sm:modal-middle"
      role="dialog"
      aria-modal="true"
      aria-labelledby="verification-findings-title"
      aria-describedby="verification-findings-description"
      @close="restoreTriggerFocus"
    >
      <div class="modal-box flex max-h-[85vh] w-full max-w-lg flex-col">
        <h3 id="verification-findings-title" class="text-lg font-bold">Template verification</h3>
        <p id="verification-findings-description" class="mt-2 text-sm text-base-content/70">
          Approval is available only when verification returns no findings.
        </p>

        <div v-if="isVerifying" class="my-5 flex items-center gap-3" role="status">
          <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
          Verifying template…
        </div>

        <div v-else-if="error" class="my-5 alert alert-error" role="alert">
          <span>{{ error }}</span>
          <button type="button" class="btn btn-sm" @click="verifyTemplate">Retry</button>
        </div>

        <div v-else-if="verificationSucceeded && findings.length > 0" class="my-5">
          <div class="alert alert-warning">
            Verification returned {{ findings.length }} finding{{ findings.length === 1 ? '' : 's' }}. Resolve every
            finding before approving.
          </div>
          <ul class="mt-3 max-h-72 space-y-2 overflow-y-auto" aria-label="Verification findings">
            <li
              v-for="(finding, idx) in findings"
              :key="idx"
              class="rounded-box border border-base-300 bg-base-100 p-3 text-sm break-words"
            >
              {{ finding }}
            </li>
          </ul>
        </div>

        <div v-else-if="verificationSucceeded" class="my-5 alert alert-success" role="status">
          Verification completed with no findings.
        </div>

        <label v-if="verificationSucceeded && findings.length === 0" class="form-control">
          <span class="label-text mb-1">Approval comment (optional)</span>
          <textarea
            v-model="comment"
            class="textarea-bordered textarea min-h-20 w-full resize-y"
            placeholder="Add context for the approver"
          ></textarea>
        </label>

        <div class="modal-action mt-2">
          <button
            v-if="verificationSucceeded && findings.length === 0"
            type="button"
            class="btn btn-primary"
            :disabled="isConfirming"
            @click="confirmApproval"
          >
            Confirm approval
          </button>
          <button type="button" class="btn btn-outline" @click="closeModal">Cancel</button>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>
