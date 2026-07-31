<script setup lang="ts">
import { useConfirmDialog } from '@vueuse/core'
import { computed, type Ref, ref, useId, useTemplateRef, watch } from 'vue'

interface Editor {
  requiredText: boolean
  placeholder?: string
}

interface ModalData {
  message: string
  editor?: Editor
  /** Warning the user must tick a checkbox to acknowledge before confirming. */
  acknowledgement?: string
}

interface ConfirmData {
  isCanceled: boolean
  data?: string
}

const actionModal = useTemplateRef('action-modal')
const modalData: Ref<ModalData> = ref({ message: 'Confirm selection' })
const dialogTitleId = useId()
const dialogDescriptionId = useId()
const editorLabelId = useId()

const inputText = ref('')
const acknowledged = ref(false)

const inputTextId = useId()
const inputHelpId = useId()

const hasEditor = computed(() => !!modalData.value.editor)

const inputRequired = computed(() => !!modalData.value.editor?.requiredText && !inputText.value.trim())

const acknowledgementRequired = computed(() => !!modalData.value.acknowledgement && !acknowledged.value)

const confirmDisabled = computed(() => inputRequired.value || acknowledgementRequired.value)

const { isRevealed, reveal, confirm, cancel, onReveal } = useConfirmDialog<ModalData, string | undefined>()

onReveal((data) => {
  if (data) {
    modalData.value = data
  }
})

watch(isRevealed, (value) => {
  if (value) {
    inputText.value = ''
    acknowledged.value = false
    actionModal.value?.showModal()
    focusFirstControl()
  } else {
    actionModal.value?.close()
  }
})

function focusFirstControl() {
  window.requestAnimationFrame(() => {
    const dialog = actionModal.value
    if (!dialog) return

    const firstControl = dialog.querySelector<HTMLElement>('button:not([disabled]), textarea')

    if (firstControl) {
      firstControl.focus()
    } else {
      dialog.focus()
    }
  })
}

const handleConfirm = () => {
  if (confirmDisabled.value) return
  if (hasEditor.value) {
    confirm(inputText.value)
  } else {
    confirm()
  }
}

interface ModalExpose {
  reveal: (data: ModalData) => Promise<ConfirmData>
}

defineExpose<ModalExpose>({ reveal: reveal })
</script>

<template>
  <Teleport to="body">
    <dialog
      ref="action-modal"
      class="modal modal-bottom sm:modal-middle"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="dialogTitleId"
      :aria-describedby="dialogDescriptionId"
      @close="cancel"
    >
      <div class="modal-box">
        <h3 :id="dialogTitleId" class="text-lg font-bold">Confirmation</h3>
        <p :id="dialogDescriptionId" class="text-md py-4">{{ modalData.message }}</p>
        <div v-if="modalData.editor" class="mx-auto flex w-full max-w-4xl flex-col gap-2 py-3">
          <label :id="editorLabelId" class="sr-only" :for="inputTextId">Comment</label>
          <textarea
            :id="inputTextId"
            v-model="inputText"
            class="textarea mt-0.5 min-h-10 w-full resize-y rounded-lg border textarea-ghost border-base-300/50 text-sm textarea-sm"
            :placeholder="modalData.editor.placeholder ?? 'Comment'"
            :aria-invalid="inputRequired"
            :aria-describedby="inputRequired ? inputHelpId : undefined"
            rows="4"
          />
          <p v-if="inputRequired" :id="inputHelpId" class="text-xs text-error">
            A comment is required before submitting.
          </p>
        </div>
        <div
          v-if="modalData.acknowledgement"
          class="my-2 alert rounded-box alert-warning"
          data-testid="confirmation-acknowledgement"
        >
          <label class="flex cursor-pointer items-start gap-3">
            <input
              v-model="acknowledged"
              type="checkbox"
              class="checkbox mt-0.5 checkbox-sm"
              data-testid="confirmation-acknowledgement-checkbox"
            />
            <span class="text-sm font-semibold">{{ modalData.acknowledgement }}</span>
          </label>
        </div>
        <div class="modal-action flex-col" :class="{ 'flex-row-reverse justify-start': hasEditor }">
          <button
            type="button"
            class="btn btn-sm btn-primary"
            :class="{ 'btn-disabled': confirmDisabled }"
            :disabled="confirmDisabled"
            data-testid="confirmation-confirm"
            @click="handleConfirm"
          >
            {{ hasEditor ? 'Submit' : 'Confirm' }}
          </button>
          <button type="button" class="btn btn-outline btn-sm" @click="cancel">Cancel</button>
        </div>
      </div>
      <div v-if="!hasEditor" class="modal-backdrop" @click="cancel"></div>
    </dialog>
  </Teleport>
</template>
