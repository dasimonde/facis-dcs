<script setup lang="ts">
import { computed, normalizeClass, ref, useAttrs, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { type DcsContractField, fieldFillScalar } from '@/models/dcs-jsonld'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ContractState } from '@/types/contract-state'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract } from '@/models/contract/contract'

defineOptions({
  inheritAttrs: false,
})

const attrs = useAttrs()

const filteredClass = computed(() => {
  return normalizeClass(attrs.class)
    .split(' ')
    .filter(
      (cls) =>
        !['btn-primary', 'btn-secondary', 'btn-accent', 'btn-success', 'btn-warning', 'btn-error', 'btn-info'].includes(
          cls,
        ),
    )
    .join(' ')
})

const props = defineProps<{
  contract: Contract
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const { isCreator, isManager, isNegotiator } = useContractPermissions()

// SRS DCS-IR-CWE-01 / §1.2 offer→acceptance lifecycle: only a Contract Creator
// may transmit a DRAFT to the counterparty (EventOffer is allowed solely from
// DRAFT — backend command/offer.go gates on the ContractCreator role + this
// transition and derives the offerer from the caller's identity).
const canOffer = computed(() => {
  return isCreator.value && props.contract.state === ContractState.draft
})

// An OFFERED contract's only forward move is the counterparty opening the
// negotiation (contractstate.Transitions: Offered -Negotiate-> Negotiation,
// SRS §4 — the Responder may accept, negotiate or refuse). The view that does
// it exists; nothing linked to it, so a received offer showed a read-only page
// whose only enabled action was Terminate. NEGOTIATION keeps the entry so a
// round in progress is reachable the same way.
const canNegotiate = computed(() => {
  const state = props.contract.state
  return (
    (isNegotiator.value || isCreator.value) && (state === ContractState.offered || state === ContractState.negotiation)
  )
})

function openNegotiation() {
  void router.push({ name: ROUTES.CONTRACTS.NEGOTIATE, params: { did: props.contract.did } })
}

// Required contract fields still missing a filled dcs:value — the completeness
// the backend's offer gate (command/offer.go validateOfferReady, SRS §1.2
// definite proposal / §2.2.2 filled-out contract) rejects; checked here too so
// the action is disabled with a reason instead of failing on click. The
// backend stays authoritative.
const unfilledRequired = computed<DcsContractField[]>(() => {
  const fields = props.contract.contract_data?.['dcs:contractFields'] ?? []
  return fields.filter((field) => {
    if (!field['dcs:required']) return false
    const value = fieldFillScalar(field['dcs:value'])
    return value === undefined || value === null || String(value).trim() === ''
  })
})

const offerBlockedReason = computed(() => {
  if (unfilledRequired.value.length === 0) return ''
  const labels = unfilledRequired.value.map((field) => field['dcs:label'] || field['@id'])
  return `Fill the required field(s) before offering to the counterparty: ${labels.join(', ')}`
})

const canTerminate = computed(() => {
  return isManager.value && props.contract.state !== ContractState.terminated
})

// SIGNED and ACTIVE both, because deployment is a Contract Manager action and
// not only an automatic one. UC-05's stimulus is "a Contract Manager submits a
// signed contract for deployment", alongside DCS-FR-SM-12's automatic trigger on
// signing completion. Offering it only in SIGNED left the manager no way to
// re-dispatch a contract the target has to receive again — a target that was
// unreachable, lost its state, or was replaced — even though the state machine
// deliberately allows it (contractstate.EventDeploy is an idempotent ACTIVE ->
// ACTIVE re-dispatch) and the auto-deploy subscriber only logs its failures.
const canDeploy = computed(() => {
  return (
    isManager.value && (props.contract.state === ContractState.signed || props.contract.state === ContractState.active)
  )
})

const deployLabel = computed(() => (props.contract.state === ContractState.active ? 'Redeploy' : 'Deploy'))

const offering = ref(false)

const offer = async () => {
  if (!isCreator.value || props.contract.state !== ContractState.draft) return
  if (offerBlockedReason.value) return
  offering.value = true
  try {
    await contractWorkflowService.offer({
      did: props.contract.did,
      updated_at: props.contract.updated_at,
    })
    router.go(0)
  } catch (err) {
    reportActionError(err, 'Offer contract')
  } finally {
    offering.value = false
  }
}

const deploying = ref(false)

const deploy = async () => {
  if (!canDeploy.value) return
  deploying.value = true
  try {
    await contractWorkflowService.deploy({
      did: props.contract.did,
      updated_at: props.contract.updated_at,
    })
    router.go(0)
  } catch (err) {
    reportActionError(err, 'Deploy contract')
  } finally {
    deploying.value = false
  }
}

const terminate = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled, data: reason } = await confirmationModal.value.reveal({
      message: 'Proceed with terminating?',
      editor: { requiredText: true, placeholder: 'Reason' },
    })
    if (!reason) {
      reportActionError(new Error('A reason is required.'), 'Terminate contract')
      return
    }
    if (!isCanceled) {
      const response = await contractWorkflowService.terminate({
        did: props.contract.did,
        updated_at: props.contract.updated_at,
        reason: reason,
      })
      if (response.did) {
        await router.push({ name: ROUTES.CONTRACTS.LIST })
      }
    }
  } catch (err) {
    reportActionError(err, 'Terminate contract')
  }
}
</script>

<template>
  <button
    v-if="canOffer"
    :class="[filteredClass, 'btn-primary']"
    :disabled="offering || offerBlockedReason !== ''"
    :title="offerBlockedReason || undefined"
    @click="offer"
  >
    {{ offering ? 'Offering…' : 'Offer to counterparty' }}
  </button>
  <button
    v-if="canDeploy"
    data-testid="deploy-contract"
    :class="[filteredClass, 'btn-primary']"
    :disabled="deploying"
    @click="deploy"
  >
    {{ deploying ? 'Deploying…' : deployLabel }}
  </button>
  <button
    v-if="canNegotiate"
    data-testid="open-negotiation"
    :class="[filteredClass, 'btn-primary']"
    @click="openNegotiation"
  >
    Negotiate
  </button>
  <button v-if="canTerminate" :class="[filteredClass, 'btn-error']" @click="terminate">Terminate</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
