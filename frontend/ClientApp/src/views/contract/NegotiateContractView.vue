<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import WorkflowStageBanner from '@core/components/WorkflowStageBanner.vue'
import { contractStory, toBannerActions } from '@core/workflow-story'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import DataObjectsEditor from '@template-repository/components/data-objects/DataObjectsEditor.vue'
import { buildContractDocument } from '@template-repository/store/dcsDraftStore'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import AuditView from '@contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@contract-workflow-engine/components/ContractDetailsEditor.vue'
import ContractHistoryDiffView from '@contract-workflow-engine/components/ContractHistoryDiffView.vue'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import {
  collectDeclaredRequirements,
  fromDocumentSemanticValues,
} from '@contract-workflow-engine/utils/semantic-condition-values'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import ContractProposalReview from '@/components/contract/ContractProposalReview.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { resolveLocalInstanceDid } from '@/services/did-service'
import { useAuthStore } from '@/stores/auth-store'
import { useErrorStore } from '@/stores/error-store'
import { useNavStore } from '@/stores/nav-store'
import { contractAuditRoles } from '@/types/contract-audit-roles'
import { ContractState } from '@/types/contract-state'
import {
  acceptedNegotiations,
  activeNegotiations,
  contractWithAcceptedProposals,
  hasUnresolvedProposals,
  locallyActionableNegotiations,
} from '@/utils/contract-negotiations'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract, ContractChangeRequest } from '@/models/contract/contract'
import type { ContractData, SemanticConditionValue } from '@/models/contract/contract-data'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

const route = useRoute()
const navStore = useNavStore()

const authStore = useAuthStore()
const issuer = computed(() => authStore.user?.issuer)

const errorStore = useErrorStore()

const dcsDraftStore = useDcsDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)
const contractContentValuesStore = useContractContentValuesStore()

const isSubmitting = ref(false)

const { isCreator, isReviewer, isManager, isNegotiator } = useContractPermissions()

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const isAuditingAuthorized = computed(
  () => contractAuditRoles.some((role) => authStore.user?.roles?.includes(role)) ?? false,
)

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft, [
    'details',
    'content',
    'diff',
    'proposals',
    'audit',
  ]),
)

const activeProposalCount = computed(() => activeNegotiations(contract.value).length)

const story = computed(() =>
  contractStory(contract.value?.state, { extrinsicLifecycle: contract.value?.extrinsic_lifecycle }),
)

const verificationResult = computed(() => {
  return verifySemanticValue(
    dcsDraftStore.semanticConditions,
    contractContentValuesStore.semanticConditionValues,
    dcsDraftStore.blocks,
  )
})

const contract: Ref<Contract | null> = ref(null)
const editedContract: Ref<Contract | null> = ref(null)

// Decisions are keyed by this party's DCS instance did:web, not by the
// logged-in user's issuer (that is the signatory's organization). Declared
// before loadContract — the watch(immediate) path calls loadContract during
// setup, so a later const would hit the temporal dead zone.
const localInstanceDid = ref('')

// Formal row plus every fully accepted proposal for this version. Details/
// Content and further Change Proposals start from this; the stored formal
// contract is unchanged until Submit merges.
const negotiationBaseline = computed(() => (contract.value ? contractWithAcceptedProposals(contract.value) : null))

const hasAcceptedProposals = computed(() => acceptedNegotiations(contract.value).length > 0)

// Comment to the other party, sent with the proposal (DCS-IR-CWE-03).
const proposalComment = ref('')

const hasChangeRequest = computed(() => {
  return (
    !!proposalComment.value.trim() ||
    changedName.value ||
    changedDescription.value ||
    changedContractData.value ||
    changedStartDate.value ||
    changedExpDate.value ||
    changeExpNoticePeriod.value ||
    changeExpPolicy.value
  )
})

// Global Name and Base Description must be non-empty.
const detailsComplete = computed(
  () => !!editedContract.value?.name?.trim() && !!editedContract.value?.description?.trim(),
)
const detailsValidationAttempted = ref(false)
const nameError = computed(() =>
  detailsValidationAttempted.value && !editedContract.value?.name?.trim() ? 'Global Name is required.' : null,
)
const descriptionError = computed(() =>
  detailsValidationAttempted.value && !editedContract.value?.description?.trim()
    ? 'Base Description is required.'
    : null,
)

const contractSemanticConditionValueSnapshot: Ref<SemanticConditionValue[]> = ref([])

const changedName = computed(() => editedContract.value?.name !== negotiationBaseline.value?.name)
const changedDescription = computed(() => editedContract.value?.description !== negotiationBaseline.value?.description)
const changedStartDate = computed(() => editedContract.value?.start_date !== negotiationBaseline.value?.start_date)
const changedExpDate = computed(() => editedContract.value?.exp_date !== negotiationBaseline.value?.exp_date)
const changeExpNoticePeriod = computed(
  () => editedContract.value?.exp_notice_period != negotiationBaseline.value?.exp_notice_period,
)
const changeExpPolicy = computed(() => editedContract.value?.exp_policy != negotiationBaseline.value?.exp_policy)
const changedContractData = computed(() => {
  const storedValues = contractContentValuesStore.semanticConditionValues
  return !semanticConditionValuesEqual(storedValues, contractSemanticConditionValueSnapshot.value)
})

const semanticConditionValuesEqual = (a: SemanticConditionValue[], b: SemanticConditionValue[]) => {
  if (a.length !== b.length) return false
  const bValues = new Map(
    b.map((value) => [`${value.blockId}|${value.conditionId}|${value.parameterName}`, value.parameterValue]),
  )
  return a.every((value) => {
    const key = `${value.blockId}|${value.conditionId}|${value.parameterName}`
    return bValues.get(key) === value.parameterValue
  })
}

function buildCurrentContractData(): ContractData | undefined {
  if (!contract.value) return undefined
  return buildContractDocument({
    documentId:
      ((contract.value.contract_data as Record<string, unknown> | undefined)?.['@id'] as string | undefined) ??
      contract.value.did,
    storedDocument: contract.value.contract_data,
    name: editedContract.value?.name ?? contract.value.name,
    description: editedContract.value?.description ?? contract.value.description,
    blocks: dcsDraftStore.blocks,
    layout: dcsDraftStore.layout,
    contractFields: dcsDraftStore.contractFields,
    contractData: dcsDraftStore.contractData,
    policies: dcsDraftStore.policies,
    semanticConditionValues: contractContentValuesStore.semanticConditionValues,
    derivedFromTemplate: contract.value.contract_data?.derivedFromTemplate,
    parentContractDid: contract.value.contract_data?.['dcs:parentContract']?.['@id'],
  })
}

const loadContract = async () => {
  const id = route.params.did
  try {
    if (id && !Array.isArray(id)) {
      contract.value = null
      editedContract.value = null
      if (!localInstanceDid.value) {
        localInstanceDid.value = await resolveLocalInstanceDid().catch(() => '')
      }
      if (route.params.did !== id) return
      const loaded = await contractWorkflowService.retrieveById({ did: id })
      if (route.params.did !== id) return
      contract.value = loaded
      const baseline = contract.value ? contractWithAcceptedProposals(contract.value) : null
      editedContract.value = baseline ? { ...baseline } : null
      applyContractDataToDraft(baseline?.contract_data)
      await restoreNegotiationDraft()
      if (route.params.did !== id) return
      // Only jump to Proposals when this instance still owes Accept/Reject.
      // Same-version history stays listed there after handling; forcing the tab
      // for every resolved proposal left Submit disabled (gated off this tab).
      if (locallyActionableNegotiations(contract.value, localInstanceDid.value, issuer.value ?? '').length > 0) {
        contractEditorUiStore.setActiveTab('proposals')
      }
    }
  } catch (err: unknown) {
    if (route.params.did === id) reportActionError(err, 'Load negotiation')
  }
}

watch(
  () => route.params.did,
  async (value) => {
    if (!value) return
    await loadContract()
  },
  { immediate: true },
)

watch(
  () => [dcsDraftStore.blocks, dcsDraftStore.semanticConditions, dcsDraftStore.contractData],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(
          conditionValue,
          dcsDraftStore.blocks,
          dcsDraftStore.semanticConditions,
          dcsDraftStore.contractData,
        ),
    )
    contractContentValuesStore.removeSemanticConditionValues(invalidValues)
  },
  { deep: true },
)

function buildChangeRequest(): ContractChangeRequest {
  const changeRequest: ContractChangeRequest = {}
  if (!editedContract.value) return changeRequest
  if (changedName.value) {
    changeRequest.name = editedContract.value.name
  }
  if (changedDescription.value) {
    changeRequest.description = editedContract.value.description
  }
  if (changedStartDate.value) {
    changeRequest.start_date = editedContract.value.start_date
  }
  if (changedExpDate.value) {
    changeRequest.exp_date = editedContract.value.exp_date
  }
  if (changeExpNoticePeriod.value) {
    changeRequest.exp_notice_period = editedContract.value.exp_notice_period
  }
  if (changeExpPolicy.value) {
    changeRequest.exp_policy = editedContract.value.exp_policy
  }
  if (changedContractData.value) {
    changeRequest.contract_data = buildCurrentContractData()
  }
  if (proposalComment.value.trim()) {
    changeRequest.comment = proposalComment.value.trim()
  }
  return changeRequest
}

const negotiateContractChange = async () => {
  if (!contract.value || !editedContract.value || !issuer.value) return
  detailsValidationAttempted.value = true
  if (!detailsComplete.value) {
    contractEditorUiStore.setActiveTab('details')
    return
  }
  // Same gate the forward-to-approval path applies: a counter-offer breaking the
  // contract's machine-readable policy is refused at approval anyway, so it is
  // reported here — naming the constraint — instead of shipping to the peer.
  if (!verificationResult.value.isValid) {
    verificationResult.value.errors.forEach((error) => errorStore.add(error.message))
    contractEditorUiStore.setActiveTab('content')
    return
  }
  isSubmitting.value = true
  try {
    const response = await contractWorkflowService.negotiate({
      did: contract.value?.did,
      updated_at: contract.value?.updated_at,
      negotiated_by: issuer.value,
      change_request: buildChangeRequest(),
    })
    if (response.did) {
      proposalComment.value = ''
      // Proposing consumed the server-side draft (SRS §3.1.1 "Save draft" vs
      // "Propose change") — loadContract re-checks and finds none.
      await loadContract()
    }
  } catch (err) {
    reportActionError(err, 'Propose contract changes')
  } finally {
    isSubmitting.value = false
  }
}

// Accepting an offered contract as it stands (SRS §4: the Responder may accept,
// negotiate or refuse). This is what mints this instance's negotiation task for
// the round, so it is also what makes the contract appear in the Negotiations
// tab and what Submit later reads as "a party engaged".
const acceptOffer = async () => {
  if (!contract.value || !issuer.value) return
  isSubmitting.value = true
  try {
    const response = await contractWorkflowService.acceptOffer({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      accepted_by: issuer.value,
    })
    if (response.did) {
      await loadContract()
    }
  } catch (err) {
    reportActionError(err, 'Accept offer')
  } finally {
    isSubmitting.value = false
  }
}

// SRS §3.1.1 Contract Negotiation UI "Save draft": stage the current
// modifications privately (per contract + author, server-side) without
// proposing them; restored into the editor on the next visit.
const draftSaved = ref(false)
const isSavingDraft = ref(false)

const saveNegotiationDraft = async () => {
  if (!contract.value || !hasChangeRequest.value) return
  isSavingDraft.value = true
  try {
    await contractWorkflowService.saveNegotiationDraft({
      did: contract.value.did,
      change_request: buildChangeRequest(),
    })
    draftSaved.value = true
    errorStore.add('Negotiation draft saved. It stays private until you propose it.', 'info')
  } catch (err) {
    reportActionError(err, 'Save negotiation draft')
  } finally {
    isSavingDraft.value = false
  }
}

const discardNegotiationDraft = async () => {
  if (!contract.value) return
  isSavingDraft.value = true
  try {
    await contractWorkflowService.deleteNegotiationDraft({ did: contract.value.did })
    draftSaved.value = false
    await loadContract()
  } catch (err) {
    reportActionError(err, 'Discard negotiation draft')
  } finally {
    isSavingDraft.value = false
  }
}

async function restoreNegotiationDraft() {
  if (!contract.value || !editedContract.value) return
  const currentContract = contract.value
  try {
    const response = await contractWorkflowService.retrieveNegotiationDraft({ did: currentContract.did })
    if (contract.value !== currentContract || route.params.did !== currentContract.did || !editedContract.value) return
    const changeRequest = response?.change_request
    if (!changeRequest) {
      draftSaved.value = false
      return
    }
    draftSaved.value = true
    if (changeRequest.name !== undefined) editedContract.value.name = changeRequest.name
    if (changeRequest.description !== undefined) editedContract.value.description = changeRequest.description
    if (changeRequest.start_date !== undefined) editedContract.value.start_date = changeRequest.start_date
    if (changeRequest.exp_date !== undefined) editedContract.value.exp_date = changeRequest.exp_date
    if (changeRequest.exp_notice_period !== undefined)
      editedContract.value.exp_notice_period = changeRequest.exp_notice_period
    if (changeRequest.exp_policy !== undefined) editedContract.value.exp_policy = changeRequest.exp_policy
    if (changeRequest.comment !== undefined) proposalComment.value = changeRequest.comment
    if (changeRequest.contract_data) {
      // Same mechanism the negotiation compare view uses to overlay a change
      // request's values onto the editor.
      const draftValues = fromDocumentSemanticValues(collectDeclaredRequirements(changeRequest.contract_data))
      draftValues.forEach((value) => contractContentValuesStore.setSemanticConditionValue(value))
    }
  } catch (err) {
    if (contract.value === currentContract && route.params.did === currentContract.did)
      reportActionError(err, 'Restore negotiation draft')
  }
}

const submitContract = async () => {
  if (!contract.value) return
  isSubmitting.value = true
  try {
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
    })
    if (response.did) {
      if (response.current_state !== contract.value.state) {
        await navStore.goToPreviousRoute()
      } else {
        const otherNegotiatorsCount = (contract.value.responsible?.negotiators.length ?? 0) - 1
        errorStore.add(`Awaiting approvals from ${otherNegotiatorsCount} other negotiators.`, 'info')
        await loadContract()
      }
    }
  } catch (err) {
    reportActionError(err, 'Submit negotiated contract')
  } finally {
    isSubmitting.value = false
  }
}

// Submit waits until every proposal is handled (peer Accept/Reject on ours,
// or our Accept/Reject on theirs). Author "Not required" nulls do not count.
const hasUnresolvedRound = computed(() =>
  hasUnresolvedProposals(contract.value, localInstanceDid.value, issuer.value ?? ''),
)

const pendingNegotiationSync = computed(() => !!contract.value?.pending_negotiation_sync)

// Accept offer is counterparty-only (design accept_offer refuses the origin).
// Showing it on the offering instance after we always open OFFERED tasks on
// this page was confusing — they already sent the offer.
const canAcceptOffer = computed(() => {
  if (contract.value?.state !== ContractState.offered || !localInstanceDid.value) return false
  const counterparty = contract.value.responsible?.counterparty
  return !!counterparty && counterparty === localInstanceDid.value
})

// Counterparty edits only in NEGOTIATION. The origin may also edit while still
// OFFERED (after Offer, before/while the peer opens the round) so it is not
// stuck read-only waiting for the counterparty's first Change Proposal.
const formEditable = computed(() => {
  const state = contract.value?.state
  if (state === ContractState.negotiation) return true
  if (state !== ContractState.offered || !localInstanceDid.value) return false
  const originPeer = contract.value?.responsible?.creator ?? contract.value?.created_by
  return originPeer === localInstanceDid.value
})

// Own proposals whose counterparty decision is still missing or undecided —
// inbound Accept/Reject from the peer does not flip pending_negotiation_sync
// (that flag is only our outbound queue), so Active proposals would stay stale
// after Submit already unlocks for the author.
const awaitingPeerProposalDecision = computed(() => {
  if (!contract.value || !issuer.value || !localInstanceDid.value) return false
  return (contract.value.negotiations ?? []).some((negotiation) => {
    if (negotiation.created_by !== issuer.value) return false
    const peerDecisions = negotiation.negotiation_decisions.filter(
      (decision) => decision.negotiator !== localInstanceDid.value,
    )
    if (peerDecisions.length === 0) return true
    return peerDecisions.some((decision) => !decision.decision)
  })
})

const shouldPollNegotiationUpdates = computed(() => pendingNegotiationSync.value || awaitingPeerProposalDecision.value)

// Re-fetch a few times so Accept/Submit unlock and Active proposals pick up the
// peer's Accept without a manual Refresh. Leave the page cancels the timers.
const negotiationSyncPollTimers: ReturnType<typeof setTimeout>[] = []

function clearNegotiationSyncPoll() {
  while (negotiationSyncPollTimers.length > 0) {
    clearTimeout(negotiationSyncPollTimers.pop())
  }
}

async function refreshNegotiationSyncStatus() {
  const id = route.params.did
  if (!id || Array.isArray(id) || !contract.value) return
  try {
    const latest = await contractWorkflowService.retrieveById({ did: id })
    if (!latest || route.params.did !== id || contract.value?.did !== id) return
    // Only sync gate + negotiation rows — do not reset the editor mid-edit.
    contract.value = {
      ...contract.value,
      pending_negotiation_sync: latest.pending_negotiation_sync,
      negotiations: latest.negotiations,
      updated_at: latest.updated_at,
    }
  } catch {
    // Next scheduled poll retries; leave the current gate state as-is.
  }
}

function scheduleNegotiationSyncPoll() {
  clearNegotiationSyncPoll()
  if (!shouldPollNegotiationUpdates.value) return
  for (const delayMs of [3000, 5000, 10000]) {
    negotiationSyncPollTimers.push(
      setTimeout(() => {
        void refreshNegotiationSyncStatus().then(() => {
          if (!shouldPollNegotiationUpdates.value) clearNegotiationSyncPoll()
        })
      }, delayMs),
    )
  }
}

watch(shouldPollNegotiationUpdates, (shouldPoll) => {
  if (shouldPoll) scheduleNegotiationSyncPoll()
  else clearNegotiationSyncPoll()
})

onMounted(async () => {
  templateEditorUiStore.reset({ workflow: 'contract' })
  // Our own party identity, used to tell our pending decisions from the peer's.
  // Resolve before the first actionable-tab check on reload after respond.
  if (!localInstanceDid.value) {
    localInstanceDid.value = await resolveLocalInstanceDid().catch(() => '')
  }
  if (locallyActionableNegotiations(contract.value, localInstanceDid.value, issuer.value ?? '').length > 0) {
    contractEditorUiStore.setActiveTab('proposals')
  }
})

onUnmounted(() => {
  clearNegotiationSyncPoll()
  dcsDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    contractSemanticConditionValueSnapshot.value = []
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    return
  }
  const cd = preprocessContractData(contractData)
  contractSemanticConditionValueSnapshot.value = (cd?.semanticConditionValues ?? []).map((value) => ({ ...value }))
  if (cd) {
    dcsDraftStore.reset({
      workflow: 'contract',
      documentIri: ((contractData as Record<string, unknown>)['@id'] as string | undefined) ?? null,
      blocks: cd.blocks,
      layout: cd.layout,
      contractFields: cd.contractFields,
      contractData: cd.contractData,
      policies: cd.policies,
    })
    contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  } else {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
  }
}

const handleNegotiationResponded = async () => {
  await loadContract()
}

const shownData = computed(() => {
  if (!!editedContract.value) {
    return editedContract.value
  }
  return contract.value
})

const currentContractData = computed<ContractData | undefined>(() => {
  return buildCurrentContractData()
})

const { download: downloadExport, exporting } = useDocumentExport()

const exportPDF = async () => {
  const did = contract?.value?.did
  if (!did) return
  await downloadExport(() => contractWorkflowService.exportPdf(did), `contract-${did}.pdf`)
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div v-if="!!contract && !!editedContract && !!shownData">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">Negotiate Contract</p>
            <div role="tablist" class="tabs-border tabs tabs-lg">
              <button
                v-for="tab in tabs"
                :key="tab.id"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.id"
                class="tab text-base-content/70"
                :class="{ 'tab-active text-primary': activeTab === tab.id }"
                @click="contractEditorUiStore.setActiveTab(tab.id)"
              >
                {{ tab.label }}
                <span
                  v-if="tab.id === 'proposals' && activeProposalCount > 0"
                  class="ml-2 badge badge-sm badge-warning"
                >
                  {{ activeProposalCount }}
                </span>
              </button>
            </div>
          </div>
        </div>
        <!-- Tab content -->
        <div class="mt-5 grow">
          <div class="mx-auto max-w-4xl p-6">
            <div class="grid grid-cols-1 gap-4">
              <WorkflowStageBanner
                :steps="story.steps"
                :current-key="story.currentKey"
                :headline="story.headline"
                :narrative="story.narrative"
                :actions="toBannerActions(story.actionHints)"
              />

              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor
                  :contract="shownData"
                  :disabled="!formEditable"
                  :name-error="nameError ?? undefined"
                  :description-error="descriptionError ?? undefined"
                />
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div class="space-y-5">
                      <TemplatePreview
                        :layout="dcsDraftStore.layout"
                        :blocks="dcsDraftStore.blocks"
                        :semantic-conditions="dcsDraftStore.semanticConditions"
                        :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                        :verification-result="verificationResult"
                        :set-semantic-condition-value="formEditable ? setSemanticConditionValue : undefined"
                        :disabled="!formEditable"
                      />
                      <template v-if="dcsDraftStore.contractData.length">
                        <div class="divider text-xs text-base-content/40">semantic data objects</div>
                        <DataObjectsEditor
                          mode="contract"
                          :editable="formEditable && !!setSemanticConditionValue"
                          :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                          :set-semantic-condition-value="
                            formEditable ? (setSemanticConditionValue ?? undefined) : undefined
                          "
                        />
                      </template>
                    </div>
                  </div>
                </div>
              </div>

              <div v-show="activeTab === 'diff'">
                <ContractHistoryDiffView
                  v-if="contract"
                  :contract-did="contract.did"
                  :contract-state="contract.state"
                  :current-contract-data="currentContractData"
                  :current-details="{
                    name: shownData?.name,
                    description: shownData?.description,
                    contract_version: contract.contract_version,
                    start_date: shownData?.start_date,
                    exp_date: shownData?.exp_date,
                    exp_policy: shownData?.exp_policy,
                    exp_notice_period: shownData?.exp_notice_period,
                  }"
                  :current-option-label="
                    hasAcceptedProposals ? (hasChangeRequest ? 'Current draft' : 'Accepted changes') : undefined
                  "
                  :formal-contract-data="hasAcceptedProposals ? contract.contract_data : undefined"
                  :formal-details="
                    hasAcceptedProposals
                      ? {
                          name: contract.name,
                          description: contract.description,
                          contract_version: contract.contract_version,
                          start_date: contract.start_date,
                          exp_date: contract.exp_date,
                          exp_policy: contract.exp_policy,
                          exp_notice_period: contract.exp_notice_period,
                        }
                      : null
                  "
                  :formal-option-label="
                    hasAcceptedProposals ? `Formal contract (version ${contract.contract_version})` : undefined
                  "
                />
              </div>

              <div v-show="activeTab === 'proposals'">
                <ContractProposalReview
                  v-if="contract"
                  :contract="contract"
                  :local-instance-did="localInstanceDid"
                  @responded="handleNegotiationResponded"
                  @refresh="handleNegotiationResponded"
                />
              </div>

              <template v-if="isAuditingAuthorized">
                <div v-show="activeTab === 'audit'">
                  <div class="card border border-base-300 bg-base-100 shadow-sm">
                    <div class="card-body">
                      <h2 class="card-title text-sm">Audit History</h2>
                      <AuditView
                        v-if="contract"
                        :contract="contract"
                        :blocks="dcsDraftStore.blocks"
                        :layout="dcsDraftStore.layout"
                      />
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div v-if="formEditable && activeTab !== 'proposals'" class="mx-auto max-w-4xl px-6 pt-3">
        <label class="floating-label w-full">
          <span>Comment to the other party</span>
          <textarea
            v-model="proposalComment"
            class="textarea w-full"
            rows="2"
            placeholder="Comment to the other party (optional)"
            data-testid="proposal-comment-input"
          ></textarea>
        </label>
      </div>
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <!-- Needs the loaded contract's DID; until it arrives exportPDF can only
             return silently, so the click looks like it did nothing. -->
        <button class="btn btn-outline md:w-32" :disabled="exporting || !contract" @click="exportPDF">
          Export PDF
        </button>
        <button
          v-if="formEditable"
          class="btn btn-outline md:w-36"
          :disabled="
            (!isCreator && !isNegotiator && !isReviewer && !isManager) ||
            isSavingDraft ||
            isSubmitting ||
            !hasChangeRequest ||
            activeTab === 'proposals'
          "
          @click="saveNegotiationDraft"
        >
          <span v-if="isSavingDraft" class="loading loading-sm loading-spinner"></span>
          Save draft
        </button>
        <button
          v-if="draftSaved && formEditable"
          class="btn btn-outline md:w-36"
          :disabled="isSavingDraft || isSubmitting"
          @click="discardNegotiationDraft"
        >
          Discard draft
        </button>
        <button
          v-if="canAcceptOffer"
          data-testid="accept-offer"
          class="btn flex-1 btn-primary"
          :disabled="(!isCreator && !isNegotiator && !isManager) || isSubmitting"
          @click="acceptOffer"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Accept offer
        </button>
        <button
          v-if="formEditable"
          class="btn flex-1 btn-primary"
          data-testid="change-proposal"
          :disabled="
            (!isCreator && !isNegotiator && !isReviewer && !isManager) ||
            isSubmitting ||
            !hasChangeRequest ||
            activeTab === 'proposals'
          "
          @click="negotiateContractChange"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Change Proposal
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn flex-1 btn-primary"
          data-testid="submit-negotiation"
          :disabled="
            (!isCreator && !isReviewer && !isNegotiator && !isManager) ||
            isSubmitting ||
            hasChangeRequest ||
            hasUnresolvedRound ||
            pendingNegotiationSync
          "
          :title="
            pendingNegotiationSync
              ? 'Waiting for negotiation messages to reach the peer.'
              : hasUnresolvedRound
                ? 'Waiting for every proposal to be accepted or rejected.'
                : undefined
          "
          @click="submitContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          {{ hasAcceptedProposals ? 'Submit' : 'Submit review' }}
        </button>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
    </div>
  </div>
</template>
