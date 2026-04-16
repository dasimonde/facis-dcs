<script setup lang="ts">
import { useConfirmDialog } from '@vueuse/core'
import { computed, ref, useTemplateRef, watch, type Ref } from 'vue'

interface ModalData {
  message: string
  requiredText?: boolean
}

interface ConfirmData {
  isCanceled: boolean
  data?: string
}

const props = defineProps<{
  showEditor?: boolean
  editorPlaceholder?: 'Comment' | 'Decision Note'
}>()

const actionModal = useTemplateRef('action-modal')
const modalData: Ref<ModalData> = ref({ message: 'Confirm selection' })

const inputText = ref('')

const hasEditor = computed(() => !!props.showEditor)

const inputRequired = computed(() => !!modalData.value.requiredText && !inputText.value.trim())

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
  if (hasEditor) {
    if (inputRequired.value) return
    confirm(inputText.value)
  } else {
    confirm()
  }
}

defineExpose({ reveal: reveal as (data: ModalData) => Promise<ConfirmData> })
</script>

<template>
  <dialog ref="action-modal" @close="cancel" class="modal modal-bottom sm:modal-middle">
    <div class="modal-box">
      <h3 class="text-lg font-bold">Confirmation</h3>
      <p class="text-md py-4">{{ modalData.message }}</p>
      <div v-if="showEditor" class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <textarea
          v-model="inputText"
          class="textarea textarea-ghost textarea-sm w-full mt-0.5 text-sm min-h-10 resize-y border border-base-300/50 rounded-lg"
          :placeholder="editorPlaceholder ?? 'Comment'"
          rows="4"
        />
      </div>
      <div class="modal-action flex-col" :class="{ 'flex-row-reverse justify-start': hasEditor }">
        <button
          class="btn btn-soft btn-sm btn-primary"
          :class="{ 'btn-disabled': inputRequired }"
          @click="handleConfirm"
        >
          {{ hasEditor ? 'Submit' : 'Confirm' }}
        </button>
        <button class="btn btn-soft btn-sm btn-error" @click="cancel">Cancel</button>
      </div>
    </div>
    <div v-if="!hasEditor" class="modal-backdrop" @click="cancel"></div>
  </dialog>
</template>
