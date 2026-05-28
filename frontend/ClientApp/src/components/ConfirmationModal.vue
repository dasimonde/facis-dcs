<script setup lang="ts">
import { useConfirmDialog } from '@vueuse/core'
import { computed, ref, useTemplateRef, watch, type Ref } from 'vue'

interface Editor {
  requiredText: boolean
  placeholder?: string
}

interface ModalData {
  message: string
  editor?: Editor
}

interface ConfirmData {
  isCanceled: boolean
  data?: string
}

const actionModal = useTemplateRef('action-modal')
const modalData: Ref<ModalData> = ref({ message: 'Confirm selection' })

const inputText = ref('')

const hasEditor = computed(() => !!modalData.value.editor)

const inputRequired = computed(() => !!modalData.value.editor?.requiredText && !inputText.value.trim())

const { isRevealed, reveal, confirm, cancel, onReveal } = useConfirmDialog<ModalData, string | undefined>()

onReveal((data) => {
  if (data) {
    modalData.value = data
  }
})

watch(isRevealed, (value) => {
  if (value) {
    inputText.value = ''
    actionModal.value?.showModal()
  } else {
    actionModal.value?.close()
  }
})

const handleConfirm = () => {
  if (hasEditor.value) {
    if (inputRequired.value) return
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
  <dialog ref="action-modal" class="modal modal-bottom sm:modal-middle" @close="cancel">
    <div class="modal-box">
      <h3 class="text-lg font-bold">Confirmation</h3>
      <p class="text-md py-4">{{ modalData.message }}</p>
      <div v-if="modalData.editor" class="mx-auto flex max-w-4xl flex-col gap-3 py-3 md:flex-row">
        <textarea
          v-model="inputText"
          class="textarea mt-0.5 min-h-10 w-full resize-y rounded-lg border textarea-ghost border-base-300/50 text-sm textarea-sm"
          :placeholder="modalData.editor.placeholder ?? 'Comment'"
          rows="4"
        />
      </div>
      <div class="modal-action flex-col" :class="{ 'flex-row-reverse justify-start': hasEditor }">
        <button class="btn btn-sm btn-primary" :class="{ 'btn-disabled': inputRequired }" @click="handleConfirm">
          {{ hasEditor ? 'Submit' : 'Confirm' }}
        </button>
        <button class="btn btn-outline btn-sm" @click="cancel">Cancel</button>
      </div>
    </div>
    <div v-if="!hasEditor" class="modal-backdrop" @click="cancel"></div>
  </dialog>
</template>
