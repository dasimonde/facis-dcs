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
import DiffView from '@contract-workflow-engine/components/DiffView.vue'
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
import NegotiationList from '@/components/lists/contract/negotiation/NegotiationList.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { getLocalDIDFile } from '@/services/did-service'
import { useAuthStore } from '@/stores/auth-store'
import { useErrorStore } from '@/stores/error-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import { activeNegotiations } from '@/utils/contract-negotiations'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract, ContractChangeRequest } from '@/models/contract/contract'
import type { ContractData, SemanticConditionValue } from '@/models/contract/contract-data'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'
import type { ContractHistoryItem } from '@/models/responses/contract-response'
import type { UserRole } from '@/types/user-role'
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
  () =>
    (['AUDITOR', 'COMPLIANCE_OFFICER'] as UserRole[]).some((role) => authStore.user?.roles?.includes(role)) ?? false,
)

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft, [
    'details',
    'content',
    'diff',
    'audit',
  ]),
)

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

interface ProposalComparison {
  current: Contract
  proposed: Contract
  currentContractData?: ContractData
  proposedContractData?: ContractData
}

const proposalComparison = ref<ProposalComparison | null>(null)

// A structured redline is applied to the contract the moment it is proposed and
// the version it replaced is snapshotted to contract_history, keyed by the same
// contract_version the negotiation row carries. That snapshot is what the
// proposal asks to change FROM — the live contract already carries the proposal,
// so comparing against it would show the proposed document on both sides.
const supersededVersions = ref<Map<number, ContractHistoryItem>>(new Map())

const hasChangeRequest = computed(() => {
  return (
    changedName.value ||
    changedDescription.value ||
    changedContractData.value ||
    changeExpNoticePeriod.value ||
    changeExpPolicy.value
  )
})

const contractSemanticConditionValueSnapshot: Ref<SemanticConditionValue[]> = ref([])

const changedName = computed(() => editedContract.value?.name !== contract.value?.name)
const changedDescription = computed(() => editedContract.value?.description !== contract.value?.description)
const changeExpNoticePeriod = computed(
  () => editedContract.value?.exp_notice_period != contract.value?.exp_notice_period,
)
const changeExpPolicy = computed(() => editedContract.value?.exp_policy != contract.value?.exp_policy)
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
  try {
    const id = route.params.did
    if (id && !Array.isArray(id)) {
      contract.value = await contractWorkflowService.retrieveById({ did: id })
      editedContract.value = !!contract.value ? { ...contract.value } : null
      applyContractDataToDraft(contract.value?.contract_data)
      const history = await contractWorkflowService.retrieveHistoryByDid({ did: id })
      supersededVersions.value = new Map(history.map((entry) => [entry.contract_version, entry]))
      await restoreNegotiationDraft()
    }
  } catch (err: unknown) {
    reportActionError(err, 'Load negotiation')
  }
}

watch(
  () => !!route.params.did,
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
  if (changeExpNoticePeriod.value) {
    changeRequest.exp_notice_period = editedContract.value.exp_notice_period
  }
  if (changeExpPolicy.value) {
    changeRequest.exp_policy = editedContract.value.exp_policy
  }
  if (changedContractData.value) {
    changeRequest.contract_data = buildCurrentContractData()
  }
  return changeRequest
}

const negotiateContractChange = async () => {
  if (!contract.value || !editedContract.value || !issuer.value) return
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
  try {
    const response = await contractWorkflowService.retrieveNegotiationDraft({ did: contract.value.did })
    const changeRequest = response?.change_request
    if (!changeRequest) {
      draftSaved.value = false
      return
    }
    draftSaved.value = true
    if (changeRequest.name !== undefined) editedContract.value.name = changeRequest.name
    if (changeRequest.description !== undefined) editedContract.value.description = changeRequest.description
    if (changeRequest.exp_notice_period !== undefined)
      editedContract.value.exp_notice_period = changeRequest.exp_notice_period
    if (changeRequest.exp_policy !== undefined) editedContract.value.exp_policy = changeRequest.exp_policy
    if (changeRequest.contract_data) {
      // Same mechanism the negotiation compare view uses to overlay a change
      // request's values onto the editor.
      const draftValues = fromDocumentSemanticValues(collectDeclaredRequirements(changeRequest.contract_data))
      draftValues.forEach((value) => contractContentValuesStore.setSemanticConditionValue(value))
    }
  } catch (err) {
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

// Only THIS party's undecided decisions may block Submit. A negotiation
// replicates to both instances carrying a decision row per negotiator, but each
// instance resolves its own row in its own database — the counterparty's
// acceptance never lands here. Counting every row therefore deadlocked the
// federated round: the peer's pending decision disabled our Submit forever, and
// responding to it matched no row (the respond updates WHERE negotiator = us).
// Identifies which negotiation decisions are OURS: decisions are keyed by the
// party's DCS instance did:web, not by the logged-in user's issuer (that is the
// signatory's organization and never matches a party).
const localInstanceDid = ref('')

// A change request we authored ourselves is excluded: FR-CWE-07 refuses an
// accept by its own author, so it would disable Submit with a decision this
// user can never resolve. The backend submit gate skips the same rows.
const hasOpenDecisions = computed(
  () =>
    contract.value?.negotiations?.some(
      (negotiation) =>
        negotiation.created_by !== issuer.value &&
        negotiation.negotiation_decisions.some(
          (decision) => !decision.decision && decision.negotiator === localInstanceDid.value,
        ),
    ) ?? false,
)

onMounted(async () => {
  templateEditorUiStore.reset({ workflow: 'contract' })
  // Our own party identity, used to tell our pending decisions from the peer's.
  localInstanceDid.value = (await getLocalDIDFile().catch(() => ({ id: '' }))).id
})

onUnmounted(() => {
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

function immutableSnapshot<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

const handleSelectedNegotiation = (negotiation: ContractNegotiation | null) => {
  if (!contract.value) return
  if (!negotiation) {
    proposalComparison.value = null
    return
  }

  const live = immutableSnapshot(contract.value)
  const superseded = supersededVersions.value.get(negotiation.contract_version)
  const current: Contract = superseded
    ? immutableSnapshot({
        ...live,
        contract_version: superseded.contract_version,
        name: superseded.name ?? live.name,
        description: superseded.description ?? live.description,
        exp_notice_period: superseded.exp_notice_period ?? live.exp_notice_period,
        exp_policy: superseded.exp_policy ?? live.exp_policy,
        contract_data: superseded.contract_data ?? live.contract_data,
      })
    : live
  const currentContractData = current.contract_data ? immutableSnapshot(current.contract_data) : undefined
  const proposedContractData = negotiation.change_request.contract_data
    ? immutableSnapshot(negotiation.change_request.contract_data)
    : currentContractData
      ? immutableSnapshot(currentContractData)
      : undefined
  const proposed = immutableSnapshot({
    ...current,
    name: negotiation.change_request.name ?? current.name,
    description: negotiation.change_request.description ?? current.description,
    exp_notice_period: negotiation.change_request.exp_notice_period ?? current.exp_notice_period,
    exp_policy: negotiation.change_request.exp_policy ?? current.exp_policy,
    contract_data: proposedContractData,
  })

  proposalComparison.value = {
    current,
    proposed,
    currentContractData,
    proposedContractData,
  }
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

// Same predicate NegotiationList renders from, so the section never appears
// around an empty list.
const hasActiveNegotiations = computed(() => activeNegotiations(contract.value).length > 0)

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
              <a
                v-for="tab in tabs"
                :key="tab.id"
                role="tab"
                class="tab text-base-content/70"
                :class="{ 'tab-active text-primary': activeTab === tab.id }"
                @click="contractEditorUiStore.setActiveTab(tab.id)"
              >
                {{ tab.label }}
              </a>
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
              <section
                v-if="proposalComparison"
                data-testid="proposal-comparison"
                class="space-y-4"
                aria-labelledby="proposal-comparison-heading"
              >
                <div>
                  <h2 id="proposal-comparison-heading" class="text-xl font-semibold">Selected change proposal</h2>
                  <p class="mt-1 text-sm text-base-content/70">
                    Current contract and proposed contract are complete, separate snapshots. Highlighting marks changed
                    contract content.
                  </p>
                </div>

                <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                  <article data-testid="proposal-current" class="card border border-base-300 bg-base-100 shadow-sm">
                    <div class="card-body min-w-0">
                      <h3 class="card-title">Current contract</h3>
                      <dl class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
                        <dt class="font-medium">Name</dt>
                        <dd class="break-words">{{ proposalComparison.current.name || '—' }}</dd>
                        <dt class="font-medium">Description</dt>
                        <dd class="break-words">{{ proposalComparison.current.description || '—' }}</dd>
                        <dt class="font-medium">Notice period</dt>
                        <dd>{{ proposalComparison.current.exp_notice_period ?? '—' }}</dd>
                        <dt class="font-medium">Expiry policy</dt>
                        <dd>{{ proposalComparison.current.exp_policy || '—' }}</dd>
                      </dl>
                    </div>
                  </article>
                  <article data-testid="proposal-proposed" class="card border border-primary/40 bg-primary/5 shadow-sm">
                    <div class="card-body min-w-0">
                      <h3 class="card-title">Proposed contract</h3>
                      <dl class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
                        <dt class="font-medium">Name</dt>
                        <dd class="break-words">{{ proposalComparison.proposed.name || '—' }}</dd>
                        <dt class="font-medium">Description</dt>
                        <dd class="break-words">{{ proposalComparison.proposed.description || '—' }}</dd>
                        <dt class="font-medium">Notice period</dt>
                        <dd>{{ proposalComparison.proposed.exp_notice_period ?? '—' }}</dd>
                        <dt class="font-medium">Expiry policy</dt>
                        <dd>{{ proposalComparison.proposed.exp_policy || '—' }}</dd>
                      </dl>
                    </div>
                  </article>
                </div>

                <DiffView
                  data-testid="proposal-content-diff"
                  :left-contract-data="proposalComparison.currentContractData"
                  :right-contract-data="proposalComparison.proposedContractData"
                  left-pane-title="Current contract content"
                  right-pane-title="Proposed contract content"
                  :show-line-numbers="true"
                  :highlight-diff="true"
                />
              </section>

              <template v-else>
                <div v-show="activeTab === 'details'">
                  <ContractDetailsEditor :contract="shownData" />
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
                          :set-semantic-condition-value="setSemanticConditionValue"
                        />
                        <template v-if="dcsDraftStore.contractData.length">
                          <div class="divider text-xs text-base-content/40">semantic data objects</div>
                          <DataObjectsEditor
                            mode="contract"
                            :editable="!!setSemanticConditionValue"
                            :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                            :set-semantic-condition-value="setSemanticConditionValue ?? undefined"
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
                  />
                </div>

                <template v-if="isAuditingAuthorized">
                  <div v-show="activeTab === 'audit'">
                    <div class="card border border-base-300 bg-base-100 shadow-sm">
                      <div class="card-body">
                        <h2 class="card-title text-sm">Audit History</h2>
                        <AuditView />
                      </div>
                    </div>
                  </div>
                </template>
              </template>
            </div>
          </div>
        </div>
      </div>
      <template v-if="activeTab !== 'audit' && hasActiveNegotiations">
        <div class="divider"></div>
        <div class="mx-auto max-w-4xl p-6">
          <div class="text-lg">Active negotiations</div>
          <NegotiationList
            :contract="contract"
            :local-instance-did="localInstanceDid"
            @selected-negotiation="handleSelectedNegotiation"
          />
        </div>
      </template>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <!-- Needs the loaded contract's DID; until it arrives exportPDF can only
             return silently, so the click looks like it did nothing. -->
        <button class="btn btn-outline md:w-32" :disabled="exporting || !contract" @click="exportPDF">
          Export PDF
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation || contract?.state === ContractState.offered"
          class="btn btn-outline md:w-36"
          :disabled="
            (!isCreator && !isNegotiator && !isReviewer && !isManager) ||
            isSavingDraft ||
            isSubmitting ||
            !hasChangeRequest ||
            !!proposalComparison
          "
          @click="saveNegotiationDraft"
        >
          <span v-if="isSavingDraft" class="loading loading-sm loading-spinner"></span>
          Save draft
        </button>
        <button
          v-if="
            draftSaved && (contract?.state === ContractState.negotiation || contract?.state === ContractState.offered)
          "
          class="btn btn-outline md:w-36"
          :disabled="isSavingDraft || isSubmitting"
          @click="discardNegotiationDraft"
        >
          Discard draft
        </button>
        <button
          v-if="contract?.state === ContractState.offered"
          data-testid="accept-offer"
          class="btn flex-1 btn-primary"
          :disabled="(!isCreator && !isNegotiator && !isManager) || isSubmitting || !!proposalComparison"
          @click="acceptOffer"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Accept offer
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation || contract?.state === ContractState.offered"
          class="btn flex-1 btn-primary"
          :disabled="
            (!isCreator && !isNegotiator && !isReviewer && !isManager) ||
            isSubmitting ||
            !hasChangeRequest ||
            !!proposalComparison
          "
          @click="negotiateContractChange"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Change Proposal
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn flex-1 btn-primary"
          :disabled="
            (!isCreator && !isReviewer && !isNegotiator) ||
            isSubmitting ||
            hasChangeRequest ||
            hasOpenDecisions ||
            !!proposalComparison
          "
          @click="submitContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Submit
        </button>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
    </div>
  </div>
</template>
