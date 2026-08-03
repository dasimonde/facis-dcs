<script setup lang="ts">
import { nextTick, ref, useTemplateRef } from 'vue'
import type { ParticipantSelection } from '@/utils/participant-selection'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    /** Contractual roles the chosen template declares, if it declares any. */
    partyRoles?: string[]
  }>(),
  { partyRoles: () => [] },
)

const emit = defineEmits<{
  submit: [value: ParticipantSelection]
}>()

const counterpartyModal = useTemplateRef<HTMLDialogElement>('counterpartyModal')
const counterparty = ref('')
const originatorRole = ref('')
const readingOrganizations = ref('')

async function openModal() {
  counterparty.value = ''
  originatorRole.value = props.partyRoles[0] ?? ''
  readingOrganizations.value = ''
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
  const parties = readingOrganizations.value
    .split(',')
    .map((name) => name.trim())
    .filter((name) => name.length > 0)
  emit('submit', {
    counterparty: counterparty.value.trim(),
    originatorRole: originatorRole.value.trim(),
    parties,
  })
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

        <label class="mt-4 flex flex-col gap-2">
          <span class="font-medium">Your role in this contract</span>
          <span class="text-xs text-base-content/70">
            Binds your organization to that role's party in the contract's machine-readable rules. The counterpart role
            stays open until the counterparty accepts by signing.
          </span>
          <select v-if="partyRoles.length" v-model="originatorRole" class="select-bordered select w-full select-sm">
            <option value="">Not declared</option>
            <option v-for="role in partyRoles" :key="role" :value="role">{{ role }}</option>
          </select>
          <input
            v-else
            v-model="originatorRole"
            type="text"
            class="input-bordered input input-sm w-full"
            placeholder="e.g. provider"
            @keydown.enter.prevent="onModalSubmit"
          />
        </label>

        <label class="mt-4 flex flex-col gap-2">
          <span class="font-medium">Organizations that may read this contract</span>
          <span class="text-xs text-base-content/70">
            Legal names, comma-separated, matched against the organization in the reader's credential. Your own
            organization always has access.
          </span>
          <input
            v-model="readingOrganizations"
            type="text"
            class="input-bordered input input-sm w-full"
            placeholder="Acme GmbH, Beispiel AG"
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
