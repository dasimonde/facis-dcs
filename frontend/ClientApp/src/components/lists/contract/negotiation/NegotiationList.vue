<script setup lang="ts">
import { computed, ref, useTemplateRef, watch } from 'vue'
import ContractDetailsDiffView from '@contract-workflow-engine/components/ContractDetailsDiffView.vue'
import DiffView from '@contract-workflow-engine/components/DiffView.vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { acceptedNegotiations, activeNegotiations, proposalComparisonBaseline } from '@/utils/contract-negotiations'
import type { Contract } from '@/models/contract/contract'
import type { ContractData } from '@/models/contract/contract-data'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'
import type { ContractNegotiationDecision } from '@/models/contract/contract-negotiation-decision'
import type { ContractHistoryItem } from '@/models/responses/contract-response'
import type { ContractDetailsSnapshot } from '@contract-workflow-engine/models/contract-details-snapshot'

const props = defineProps<{
  contract: Contract
  // This party's DCS instance did:web. Decision rows are keyed by it, not by
  // the logged-in user's issuer — that is the signatory's organization and
  // never matches a negotiator.
  localInstanceDid: string
  // Participant org id (OID4VP organization claim). Same value as negotiation created_by
  participantId?: string
  disabled?: boolean
}>()

const { isCreator, isReviewer, isManager, isNegotiator } = useContractPermissions()

// design/contract_workflow_engine.go respond(): Contract Creator, Contract
// Negotiator, Contract Reviewer, Contract Manager.
const mayRespond = computed(() => isCreator.value || isNegotiator.value || isReviewer.value || isManager.value)

const ownDecision = (negotiation: ContractNegotiation) =>
  negotiation.negotiation_decisions.find((decision) => decision.negotiator === props.localInstanceDid)

// Accept/Reject only for the local pending decision holder who did not author
// the proposal. Everyone else can still Show/Hide the comparison.
const canRespondTo = (negotiation: ContractNegotiation) => {
  if (props.disabled || !mayRespond.value) return false
  if (props.participantId && negotiation.created_by === props.participantId) return false
  const decision = ownDecision(negotiation)
  return decision != null && decision.decision == null
}

const isOwnProposal = (negotiation: ContractNegotiation) =>
  !!props.participantId && negotiation.created_by === props.participantId

// Author may withdraw only while the proposal is still open — not after a
// counterparty has Accepted or Rejected it.
const canWithdraw = (negotiation: ContractNegotiation) => {
  if (props.disabled || !mayRespond.value || !isOwnProposal(negotiation)) return false
  return !negotiation.negotiation_decisions.some(
    (decision) => decision.decision === 'ACCEPTED' || decision.decision === 'REJECTED',
  )
}

// Accept waits for earlier outbound Proposals/Decisions to be peer-ACK'd.
// Reject stays available.
const acceptBlockedBySync = computed(() => !!props.contract.pending_negotiation_sync)
const acceptBlockedReason = 'Waiting for earlier negotiation messages to reach the peer.'

const emit = defineEmits<{
  responded: []
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const sortedNegotiations = computed(() =>
  activeNegotiations(props.contract)
    .slice()
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
)

const sortedDecisions = (decisions: ContractNegotiationDecision[]) => {
  return [...decisions].sort((a, b) => a.negotiator.localeCompare(b.negotiator))
}

const isSubmitting = ref(false)

interface ProposalComparison {
  currentDetails: ContractDetailsSnapshot
  proposedDetails: ContractDetailsSnapshot
  currentContractData?: ContractData
  proposedContractData?: ContractData
  leftPaneTitle: string
  leftContentTitle: string
}

const expandedNegotiationId = ref<string | null>(null)
const comparisons = ref<Map<string, ProposalComparison>>(new Map())
const loadingComparisonId = ref<string | null>(null)
const supersededVersions = ref<Map<number, ContractHistoryItem>>(new Map())

function immutableSnapshot<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function detailsFrom(
  contract: Pick<
    Contract,
    'name' | 'description' | 'contract_version' | 'start_date' | 'exp_date' | 'exp_policy' | 'exp_notice_period'
  >,
): ContractDetailsSnapshot {
  return {
    name: contract.name,
    description: contract.description,
    contract_version: contract.contract_version,
    start_date: contract.start_date,
    exp_date: contract.exp_date,
    exp_policy: contract.exp_policy,
    exp_notice_period: contract.exp_notice_period,
  }
}

const loadSupersededVersions = async (did: string) => {
  const history = await contractWorkflowService.retrieveHistoryByDid({ did })
  supersededVersions.value = new Map(history.map((entry) => [entry.contract_version, entry]))
}

async function buildComparison(negotiation: ContractNegotiation): Promise<ProposalComparison> {
  const live = immutableSnapshot(props.contract)
  let formalBase: Contract = live
  if (negotiation.contract_version < live.contract_version) {
    if (!supersededVersions.value.has(negotiation.contract_version) && props.contract.did) {
      await loadSupersededVersions(props.contract.did)
    }
    const superseded = supersededVersions.value.get(negotiation.contract_version)
    if (superseded) {
      formalBase = immutableSnapshot({
        ...live,
        contract_version: superseded.contract_version,
        name: superseded.name ?? live.name,
        description: superseded.description ?? live.description,
        start_date: superseded.start_date ?? live.start_date,
        exp_date: superseded.exp_date ?? live.exp_date,
        exp_notice_period: superseded.exp_notice_period ?? live.exp_notice_period,
        exp_policy: superseded.exp_policy ?? live.exp_policy,
        contract_data: superseded.contract_data ?? live.contract_data,
      })
    } else {
      // History gap: keep live content for the diff, but label the proposal round.
      formalBase = immutableSnapshot({
        ...live,
        contract_version: negotiation.contract_version,
      })
    }
  }
  const current = immutableSnapshot(proposalComparisonBaseline(formalBase, negotiation))
  const priorAcceptedCount = acceptedNegotiations(formalBase).filter((entry) => {
    const byTime = new Date(entry.created_at).getTime() - new Date(negotiation.created_at).getTime()
    if (byTime !== 0) return byTime < 0
    return entry.id.localeCompare(negotiation.id) < 0
  }).length
  const leftPaneTitle = priorAcceptedCount > 0 ? 'Accepted changes' : 'Formal contract'
  const leftContentTitle = priorAcceptedCount > 0 ? 'Accepted changes content' : 'Formal contract content'
  const currentContractData = current.contract_data ? immutableSnapshot(current.contract_data) : undefined
  const changeRequest = negotiation.change_request ?? {}
  const proposedContractData = changeRequest.contract_data
    ? immutableSnapshot(changeRequest.contract_data)
    : currentContractData
      ? immutableSnapshot(currentContractData)
      : undefined
  const proposed = immutableSnapshot({
    ...current,
    name: changeRequest.name ?? current.name,
    description: changeRequest.description ?? current.description,
    start_date: changeRequest.start_date ?? current.start_date,
    exp_date: changeRequest.exp_date ?? current.exp_date,
    exp_notice_period: changeRequest.exp_notice_period ?? current.exp_notice_period,
    exp_policy: changeRequest.exp_policy ?? current.exp_policy,
    contract_data: proposedContractData,
  })

  return {
    currentDetails: detailsFrom(current),
    proposedDetails: detailsFrom(proposed),
    currentContractData,
    proposedContractData,
    leftPaneTitle,
    leftContentTitle,
  }
}

const isExpanded = (negotiationId: string) => expandedNegotiationId.value === negotiationId

const handleShowBtn = async (negotiation: ContractNegotiation) => {
  if (isExpanded(negotiation.id)) {
    expandedNegotiationId.value = null
    return
  }
  expandedNegotiationId.value = negotiation.id
  if (comparisons.value.has(negotiation.id)) return
  loadingComparisonId.value = negotiation.id
  try {
    const comparison = await buildComparison(negotiation)
    comparisons.value.set(negotiation.id, comparison)
    // Trigger reactivity for Map mutation.
    comparisons.value = new Map(comparisons.value)
  } finally {
    loadingComparisonId.value = null
  }
}

const acceptNegotiation = async (negotiation: ContractNegotiation) => {
  if (!confirmationModal.value) return
  isSubmitting.value = true
  try {
    const { isCanceled } = await confirmationModal.value?.reveal({ message: 'Accept this change request?' })
    if (!isCanceled) {
      const response = await contractWorkflowService.respond({
        id: negotiation.id,
        did: props.contract.did,
        action_flag: 'ACCEPTING',
      })
      if (response.id) {
        const decision = ownDecision(negotiation)
        if (decision) decision.decision = 'ACCEPTED'
        expandedNegotiationId.value = null
        emit('responded')
      }
    }
  } catch (err) {
    console.error('Accepting the negotiation failed', err)
  } finally {
    isSubmitting.value = false
  }
}

const rejectNegotiation = async (negotiation: ContractNegotiation) => {
  if (!confirmationModal.value) return
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
        rejection_reason: rejectResult.data,
      })
      if (response.id) {
        negotiation.negotiation_decisions.forEach((decision) => {
          if (decision.negotiator === props.localInstanceDid) {
            decision.decision = 'REJECTED'
            decision.rejection_reason = rejectResult.data
          } else {
            decision.decision = 'CLOSED'
          }
        })
        expandedNegotiationId.value = null
        emit('responded')
      }
    }
  } catch (err) {
    console.error('Rejecting the negotiation failed', err)
  } finally {
    isSubmitting.value = false
  }
}

const withdrawNegotiation = async (negotiation: ContractNegotiation) => {
  if (!confirmationModal.value) return
  isSubmitting.value = true
  try {
    const { isCanceled } = await confirmationModal.value.reveal({
      message: 'Withdraw this change proposal? The counterparty will no longer see it.',
    })
    if (!isCanceled) {
      const response = await contractWorkflowService.respond({
        id: negotiation.id,
        did: props.contract.did,
        action_flag: 'WITHDRAWING',
      })
      if (response.id) {
        expandedNegotiationId.value = null
        comparisons.value.delete(negotiation.id)
        comparisons.value = new Map(comparisons.value)
        emit('responded')
      }
    }
  } catch (err) {
    console.error('Withdrawing the negotiation failed', err)
  } finally {
    isSubmitting.value = false
  }
}

const isOwnDecision = (decision: ContractNegotiationDecision) => decision.negotiator === props.localInstanceDid

// Author never records Accept (FR-CWE-07). On the authoring instance that is
// "own proposal + own decision"; on the peer the author's replicated null row
// is "not our decision on someone else's proposal". Both must read Not required
// — otherwise B sees A's slot as PENDING forever and cannot tell the round is
// waiting on Submit, not on another Accept.
const decisionLabel = (negotiation: ContractNegotiation, decision: ContractNegotiationDecision) => {
  if (decision.decision) return decision.decision
  if (isOwnProposal(negotiation) && isOwnDecision(decision)) return 'Not required'
  if (!isOwnProposal(negotiation) && !isOwnDecision(decision)) return 'Not required'
  return 'PENDING'
}

function formatProposedAt(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString()
}

watch(
  () => props.contract.did,
  () => {
    expandedNegotiationId.value = null
    comparisons.value = new Map()
    supersededVersions.value = new Map()
  },
)

watch(
  () => props.contract.updated_at,
  () => {
    // Stale comparisons after Accept/Refresh — rebuild on next Show.
    comparisons.value = new Map()
  },
)
</script>

<template>
  <ul class="list">
    <li v-for="negotiation in sortedNegotiations" :key="negotiation.id" class="list-row px-0">
      <div class="card border-base-content/10 bg-base-100 shadow-sm card-border">
        <div class="card-body gap-3">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div class="min-w-0 space-y-1">
              <h2 class="card-title">Change proposal by: {{ negotiation.created_by }}</h2>
              <p class="text-sm text-base-content/60">Proposed {{ formatProposedAt(negotiation.created_at) }}</p>
              <p
                v-if="negotiation.change_request?.comment"
                class="text-sm whitespace-pre-wrap"
                data-testid="proposal-comment"
              >
                Comment: {{ negotiation.change_request.comment }}
              </p>
            </div>
            <span v-if="isOwnProposal(negotiation)" class="badge badge-ghost badge-sm">Your proposal</span>
            <span v-else-if="canRespondTo(negotiation)" class="badge badge-sm badge-warning">
              Awaiting your decision
            </span>
          </div>
          <div v-if="negotiation.superseded?.length" class="text-sm">
            <div class="badge badge-sm badge-warning">SUPERSEDED</div>
            <div v-for="discard in negotiation.superseded" :key="discard.superseded_by">
              Accepted, but the merged contract does not carry its {{ discard.fields.join(', ') }}. A later accepted
              change request ({{ discard.superseded_by }}) set the same.
            </div>
          </div>
          <ul class="list">
            <li
              v-for="decision in sortedDecisions(negotiation.negotiation_decisions)"
              :key="decision.negotiator"
              class="list-row px-0 py-2"
            >
              <div class="list-col-grow flex w-full justify-between gap-2">
                <div class="min-w-0 break-all">
                  {{ decision.negotiator }}
                  <span v-if="isOwnDecision(decision)" class="ml-1 badge badge-ghost badge-xs">you</span>
                </div>
                <div class="badge shrink-0 badge-sm badge-secondary">
                  {{ decisionLabel(negotiation, decision) }}
                </div>
              </div>
              <div v-if="decision.decision === 'REJECTED' && decision.rejection_reason" class="list-col-wrap truncate">
                Reason: {{ decision.rejection_reason }}
              </div>
            </li>
          </ul>

          <div
            class="grid transition-[grid-template-rows] duration-300 ease-in-out"
            :class="isExpanded(negotiation.id) ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'"
          >
            <div class="min-h-0 overflow-hidden">
              <section
                class="space-y-4 rounded-box border border-base-300 bg-base-200/20 p-4"
                :aria-hidden="!isExpanded(negotiation.id)"
              >
                <div>
                  <h3 class="text-base font-semibold">Selected change proposal</h3>
                  <p class="mt-1 text-sm text-base-content/70">
                    Left is the baseline this proposal builds on (formal contract, or accepted changes still pending
                    Submit). Right is the proposal. Highlighting marks changed fields and contract content.
                  </p>
                </div>
                <div v-if="loadingComparisonId === negotiation.id" class="flex justify-center py-6">
                  <span class="loading loading-md loading-spinner"></span>
                </div>
                <div v-else-if="comparisons.get(negotiation.id)" data-testid="proposal-comparison" class="space-y-4">
                  <ContractDetailsDiffView
                    :left="comparisons.get(negotiation.id)!.currentDetails"
                    :right="comparisons.get(negotiation.id)!.proposedDetails"
                    :left-pane-title="comparisons.get(negotiation.id)!.leftPaneTitle"
                    right-pane-title="Proposed contract"
                    left-test-id="proposal-current"
                    right-test-id="proposal-proposed"
                    :highlight-diff="true"
                  />
                  <DiffView
                    data-testid="proposal-content-diff"
                    :left-contract-data="comparisons.get(negotiation.id)!.currentContractData"
                    :right-contract-data="comparisons.get(negotiation.id)!.proposedContractData"
                    :left-pane-title="comparisons.get(negotiation.id)!.leftContentTitle"
                    right-pane-title="Proposed contract content"
                    :show-line-numbers="true"
                    :highlight-diff="true"
                  />
                </div>
              </section>
            </div>
          </div>

          <div class="card-actions justify-end">
            <button
              v-if="canWithdraw(negotiation)"
              class="btn btn-sm btn-primary"
              :disabled="isSubmitting"
              @click="withdrawNegotiation(negotiation)"
            >
              <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
              Withdraw
            </button>
            <button
              v-if="canRespondTo(negotiation) && isExpanded(negotiation.id)"
              class="btn btn-sm btn-primary"
              data-testid="accept-negotiation"
              :disabled="isSubmitting || acceptBlockedBySync"
              :title="acceptBlockedBySync ? acceptBlockedReason : undefined"
              @click="acceptNegotiation(negotiation)"
            >
              <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
              Accept
            </button>
            <p
              v-if="canRespondTo(negotiation) && isExpanded(negotiation.id) && acceptBlockedBySync"
              class="w-full text-right text-xs text-base-content/60"
              data-testid="accept-sync-pending"
            >
              {{ acceptBlockedReason }}
            </p>
            <button
              v-if="canRespondTo(negotiation) && isExpanded(negotiation.id)"
              class="btn btn-sm btn-primary"
              :disabled="isSubmitting"
              @click="rejectNegotiation(negotiation)"
            >
              <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
              Reject
            </button>
            <button class="btn btn-sm btn-primary" @click="handleShowBtn(negotiation)">
              {{ isExpanded(negotiation.id) ? 'Hide' : 'Show' }}
            </button>
          </div>
        </div>
      </div>
    </li>
  </ul>
  <ConfirmationModal ref="confirmation-modal" />
</template>
