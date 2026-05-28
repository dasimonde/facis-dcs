<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { Contract } from '@/models/contract/contract'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'
import type { ContractNegotiationDecision } from '@/models/contract/contract-negotiation-decision'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { computed, ref, useTemplateRef } from 'vue'

const props = defineProps<{
  contract: Contract
  disabled?: boolean
}>()

const emit = defineEmits<{ selectedNegotiation: [negotiation: ContractNegotiation | null] }>()

const authStore = useAuthStore()
const username = computed(() => authStore.user?.username)

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const negotiations = computed(() => {
  const activeNegotiations = props.contract.negotiations?.filter(
    (negotiation) => negotiation.contract_version === props.contract.contract_version,
  )
  return activeNegotiations ?? []
})

const sortedNegotiations = computed(() =>
  negotiations.value.slice().sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
)

const sortedDecisions = (decisions: ContractNegotiationDecision[]) => {
  return decisions.sort((a, b) => a.negotiator.localeCompare(b.negotiator))
}

const isSubmitting = ref(false)

const acceptNegotiation = async (negotiation: ContractNegotiation) => {
  if (!username.value || !confirmationModal.value) return
  isSubmitting.value = true
  try {
    const { isCanceled } = await confirmationModal.value?.reveal({ message: 'Accept this change request?' })
    if (!isCanceled) {
      const response = await contractWorkflowService.respond({
        id: negotiation.id,
        did: props.contract.did,
        action_flag: 'ACCEPTING',
        responded_by: username.value,
      })
      if (response.id) {
        const decision = negotiation.negotiation_decisions.find((decision) => decision.negotiator === username.value)
        if (decision) decision.decision = 'ACCEPTED'
      }
    }
  } catch (err) {
    console.error('Accepting the negotiation failed', err)
  } finally {
    isSubmitting.value = false
  }
}

const rejectNegotiation = async (negotiation: ContractNegotiation) => {
  if (!username.value || !confirmationModal.value) return
  isSubmitting.value = true
  try {
    const rejectResult = await confirmationModal.value.reveal({
      message: 'Reject this change request?',
      editor: { requiredText: true, placeholder: 'Rejection reason' },
    })
    if (!rejectResult.isCanceled) {
      const response = await contractWorkflowService.respond({
        id: negotiation.id,
        did: props.contract.did,
        action_flag: 'REJECTING',
        responded_by: username.value,
        rejection_reason: rejectResult.data,
      })
      if (response.id) {
        negotiation.negotiation_decisions.forEach((decision) => {
          if (decision.negotiator === username.value) {
            decision.decision = 'REJECTED'
            decision.rejection_reason = rejectResult.data
          } else {
            decision.decision = 'CLOSED'
          }
        })
      }
    }
  } catch (err) {
    console.error('Rejecting the negotiation failed', err)
  } finally {
    isSubmitting.value = false
  }
}

const isBtnDisabled = (negotiation: ContractNegotiation) => {
  const decision = negotiation.negotiation_decisions.find((decision) => decision.negotiator === username.value)
  return decision?.decision !== undefined
}

const isNegotiationShown = ref<Map<string, boolean>>(new Map())
const handleShowBtn = (negotiation: ContractNegotiation) => {
  if (!isNegotiationShown.value.has(negotiation.id)) {
    isNegotiationShown.value.forEach((_, key) => isNegotiationShown.value.delete(key))
    emit('selectedNegotiation', negotiation)
    isNegotiationShown.value.set(negotiation.id, true)
  } else {
    emit('selectedNegotiation', null)
    isNegotiationShown.value.delete(negotiation.id)
  }
}
</script>

<template>
  <ul class="list">
    <li v-for="negotiation in sortedNegotiations" :key="negotiation.id" class="list-row px-0">
      <div class="card border-base-content/10 bg-base-100 shadow-sm card-border">
        <div class="card-body">
          <h2 class="card-title">Change proposal by: {{ negotiation.created_by }}</h2>
          <ul class="list">
            <li
              v-for="decision in sortedDecisions(negotiation.negotiation_decisions)"
              :key="decision.negotiator"
              class="list-row px-0 py-2"
            >
              <div class="list-col-grow flex w-full justify-between">
                <div>{{ decision.negotiator }}</div>
                <div class="badge badge-sm badge-secondary">{{ decision.decision ?? 'PENDING' }}</div>
              </div>
              <div v-if="decision.decision === 'REJECTED' && decision.rejection_reason" class="list-col-wrap truncate">
                Reason: {{ decision.rejection_reason }}
              </div>
            </li>
          </ul>
          <div class="card-actions justify-end">
            <button
              v-if="!disabled && isNegotiationShown.get(negotiation.id)"
              class="btn btn-sm btn-primary"
              :disabled="isSubmitting || isBtnDisabled(negotiation)"
              @click="acceptNegotiation(negotiation)"
            >
              <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
              Accept
            </button>
            <button
              v-if="!disabled && isNegotiationShown.get(negotiation.id)"
              class="btn btn-sm btn-primary"
              :disabled="isSubmitting || isBtnDisabled(negotiation)"
              @click="rejectNegotiation(negotiation)"
            >
              <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
              Reject
            </button>
            <button class="btn btn-sm btn-primary" @click="handleShowBtn(negotiation)">
              {{ !isNegotiationShown.get(negotiation.id) ? 'Show' : 'Hide' }}
            </button>
          </div>
        </div>
      </div>
    </li>
  </ul>
  <ConfirmationModal ref="confirmation-modal" />
</template>
