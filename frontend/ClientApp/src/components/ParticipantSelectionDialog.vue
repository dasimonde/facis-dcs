<script setup lang="ts">
import { nextTick, ref, useTemplateRef } from 'vue'
import type { ParticipantSelection } from '@/utils/participant-selection'

defineOptions({ inheritAttrs: false })

const emit = defineEmits<{
  submit: [value: ParticipantSelection]
}>()

const counterpartyModal = useTemplateRef<HTMLDialogElement>('counterpartyModal')
const counterparty = ref('')

async function openModal() {
  counterparty.value = ''
  await nextTick()
  counterpartyModal.value?.showModal()
  focusDialog()
}

function focusDialog() {
  window.requestAnimationFrame(() => {
    counterpartyModal.value?.focus()
  })
}

function onModalSubmit() {
  emit('submit', { counterparty: counterparty.value.trim() })
  counterpartyModal.value?.close()
}

function onModalClose() {
  counterpartyModal.value?.close()
}
</script>

<template>
  <button type="button" v-bind="$attrs" @click="openModal">Create</button>
  <Teleport to="body">
    <dialog
      ref="counterpartyModal"
      class="modal modal-bottom transition-none sm:modal-middle"
      role="dialog"
      aria-modal="true"
      aria-labelledby="participant-dialog-title"
    >
      <div class="modal-box flex w-full max-w-lg flex-col">
        <h3 id="participant-dialog-title" class="text-lg font-bold">Contract Counterparty</h3>
        <p class="mt-2 mb-4 text-sm text-base-content/70">
          The other DCS this contract is offered to and negotiated with. Review, approval and negotiation are handled by
          your own instance's roles — leave empty for a purely local contract.
        </p>

        <label class="flex flex-col gap-2">
          <span class="font-medium">Counterparty did:web</span>
          <input
            v-model="counterparty"
            type="text"
            class="input-bordered input input-sm w-full font-mono text-xs"
            placeholder="did:web:..."
            @keydown.enter.prevent="onModalSubmit"
          />
        </label>

        <div class="modal-action mt-4">
          <button type="button" class="btn btn-outline" @click="onModalClose">Cancel</button>
          <button type="button" class="btn btn-primary" @click="onModalSubmit">Apply</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop">
        <button type="submit" aria-label="Close">close</button>
      </form>
    </dialog>
  </Teleport>
</template>
