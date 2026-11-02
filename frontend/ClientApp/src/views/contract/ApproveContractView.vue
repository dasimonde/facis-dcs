<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, useTemplateRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import WorkflowStageBanner from '@core/components/WorkflowStageBanner.vue'
import { contractStory, toBannerActions } from '@core/workflow-story'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import AuditView from '@contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@contract-workflow-engine/components/ContractDetailsEditor.vue'
import ContractHistoryDiffView from '@contract-workflow-engine/components/ContractHistoryDiffView.vue'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import PeerProposalPendingAlert from '@/components/contract/PeerProposalPendingAlert.vue'
import NegotiationList from '@/components/lists/contract/negotiation/NegotiationList.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { resolveLocalInstanceDid } from '@/services/did-service'
import { useAuthStore } from '@/stores/auth-store'
import { useNavStore } from '@/stores/nav-store'
import { contractAuditRoles } from '@/types/contract-audit-roles'
import { ContractState } from '@/types/contract-state'
import { hasPendingPeerProposalAfterSettle } from '@/utils/contract-negotiations'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract } from '@/models/contract/contract'
import type { ContractReviewFinding } from '@/models/responses/contract-response'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

const route = useRoute()
const navStore = useNavStore()
const authStore = useAuthStore()

const { isApprover } = useContractPermissions()

const dcsDraftStore = useDcsDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)
const contractContentValuesStore = useContractContentValuesStore()

const confirmationDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-dialog')

const isSubmitting = ref(false)

// Approve is read-only for content; proposals carry metadata changes.
const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => null)

const isAuditingAuthorized = computed(
  () => contractAuditRoles.some((role) => authStore.user?.roles?.includes(role)) ?? false,
)

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft, [
    'details',
    'content',
    'proposals',
    'diff',
    'audit',
  ]),
)

const story = computed(() =>
  contractStory(contract.value?.state, { extrinsicLifecycle: contract.value?.extrinsic_lifecycle }),
)

const verificationResult: Ref<VerificationResult | null> = ref(null)

const contract: Ref<Contract | null> = ref(null)
// The reviewer decisions, newest first, inform the approval (DCS-IR-CWE-08, DCS-FR-CWE-25).
const reviewFindings: Ref<ContractReviewFinding[]> = ref([])
const highlightedBlockIds = computed(() => reviewFindings.value[0]?.annotations.map((a) => a.block_id) ?? [])
async function loadReviewFindings(did: string) {
  reviewFindings.value = []
  try {
    const findings = await contractWorkflowService.reviewFindings(did)
    if (route.params.did === did) reviewFindings.value = findings
  } catch {
    // the shared http interceptor already surfaced the server's error
  }
}
const blockTitle = (blockId: string) => {
  const block = dcsDraftStore.blocks.find((candidate) => candidate['@id'] === blockId)
  return (block && 'dcs:title' in block ? block['dcs:title'] : undefined) ?? blockId
}
const compareChangesData: Ref<Contract | null> = ref(null)
const localInstanceDid = ref('')

const showPeerProposalAlert = computed(() => hasPendingPeerProposalAfterSettle(contract.value, localInstanceDid.value))

const settlementReady = computed(() => contract.value?.settlement_ready !== false)

const showSettlementNotReadyAlert = computed(
  () => contract.value?.state === ContractState.reviewed && !settlementReady.value,
)

watch(
  () => route.params.did,
  async (value) => {
    if (value) {
      try {
        const id = route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = null
          if (!localInstanceDid.value) {
            localInstanceDid.value = await resolveLocalInstanceDid().catch(() => '')
          }
          if (route.params.did !== id) return
          const loaded = await contractWorkflowService.retrieveById({ did: id })
          if (route.params.did !== id) return
          contract.value = loaded
          applyContractDataToDraft(contract.value?.contract_data)
          void loadReviewFindings(id)
        }
      } catch (err: unknown) {
        reportActionError(err, 'Load contract approval')
      }
    }
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

const approve = async () => {
  if (!contract.value) return
  try {
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: 'Confirm approval',
      editor: { requiredText: false, placeholder: 'Comment (optional)' },
      confirmLabel: 'Confirm',
    })
    if (confirmationResult?.isCanceled) return
    const comment = confirmationResult?.data?.trim()
    isSubmitting.value = true
    const response = await contractWorkflowService.approve({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      ...(comment ? { decision_notes: [comment] } : {}),
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    reportActionError(err, 'Approve contract')
  } finally {
    isSubmitting.value = false
  }
}

const reject = async () => {
  if (!contract.value) return
  try {
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: 'Add rejection reason',
      editor: { requiredText: true, placeholder: 'Reason' },
    })
    if (confirmationResult?.isCanceled) return
    const comment = confirmationResult?.data?.trim()
    if (!comment) {
      reportActionError(new Error('A reason is required.'), 'Reject contract')
      return
    }
    isSubmitting.value = true
    const response = await contractWorkflowService.reject({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      reason: comment,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    reportActionError(err, 'Reject contract')
  } finally {
    isSubmitting.value = false
  }
}

// REVIEWED -> SUBMITTED: the approver sends the contract back for another review
// round (DCS-IR-CWE-09).
const resubmit = async () => {
  if (!contract.value) return
  try {
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: 'Send back for review',
      editor: { requiredText: false, placeholder: 'Comment (optional)' },
    })
    if (confirmationResult?.isCanceled) return
    const comment = confirmationResult?.data?.trim()
    isSubmitting.value = true
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      ...(comment ? { comments: [comment] } : {}),
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    reportActionError(err, 'Resubmit contract')
  } finally {
    isSubmitting.value = false
  }
}

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract', isTemplateEditable: false })
})

onUnmounted(() => {
  dcsDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
  verificationResult.value = null
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    verificationResult.value = null
    return
  }
  const cd = preprocessContractData(contractData)
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
  verificationResult.value = null
}

const { download: downloadExport, exporting } = useDocumentExport()

const exportPDF = async () => {
  const did = contract?.value?.did
  if (!did) return
  await downloadExport(() => contractWorkflowService.exportPdf(did), `contract-${did}.pdf`)
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="!!contract" class="flex flex-1 flex-col">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">
              <span>Approve Contract</span>
              <span v-if="isApprover" class="font-normal normal-case">· as Contract Approver</span>
            </p>
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
              <PeerProposalPendingAlert
                :show="showPeerProposalAlert"
                message="Peer sent a change proposal. Suggest rejecting and continuing negotiation."
              />
              <PeerProposalPendingAlert
                :show="showSettlementNotReadyAlert"
                :message="
                  contract?.settlement_ready_detail || 'Waiting for matching counterparty settlement before approval.'
                "
              />
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor
                  :contract="contract"
                  :inserted="{ name: compareChangesData?.name, description: compareChangesData?.description }"
                  disabled
                />
              </div>

              <div v-show="activeTab === 'content'">
                <section
                  class="card mb-4 border border-base-300 bg-base-100 shadow-sm"
                  aria-label="Reviewer findings"
                  data-testid="reviewer-findings"
                >
                  <div class="card-body gap-3">
                    <h2 class="card-title text-sm">Reviewer findings</h2>
                    <p v-if="reviewFindings.length === 0" class="text-sm text-base-content/70">
                      No reviewer comments or annotations.
                    </p>
                    <article
                      v-for="finding in reviewFindings"
                      :key="`${finding.occurred_at}|${finding.submitted_by}`"
                      class="space-y-1 rounded-box bg-base-200 p-3"
                    >
                      <p class="text-sm font-semibold">
                        {{ finding.decision === 'REJECT' ? 'Returned to negotiation' : 'Forwarded to approval' }}
                        · version {{ finding.contract_version }} · {{ finding.submitted_by }} ·
                        {{ finding.occurred_at }}
                      </p>
                      <p v-for="(comment, index) in finding.comments" :key="`c${index}`" class="text-sm">
                        {{ comment }}
                      </p>
                      <ul v-if="finding.annotations.length" class="list-disc pl-5 text-sm">
                        <li v-for="annotation in finding.annotations" :key="annotation.block_id">
                          <span class="font-medium">{{ blockTitle(annotation.block_id) }}:</span>
                          {{ annotation.comment }}
                        </li>
                      </ul>
                    </article>
                  </div>
                </section>
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :layout="dcsDraftStore.layout"
                        :blocks="dcsDraftStore.blocks"
                        :semantic-conditions="dcsDraftStore.semanticConditions"
                        :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                        :verification-result="verificationResult"
                        :set-semantic-condition-value="setSemanticConditionValue"
                        :highlighted-block-ids="highlightedBlockIds"
                        disabled
                      />
                    </div>
                  </div>
                </div>
              </div>

              <div v-if="contract" v-show="activeTab === 'proposals'">
                <NegotiationList :contract="contract" :local-instance-did="localInstanceDid" disabled />
              </div>

              <div v-if="contract" v-show="activeTab === 'diff'">
                <ContractHistoryDiffView
                  :contract-did="contract.did"
                  :contract-state="contract.state"
                  :current-contract-data="contract.contract_data"
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
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <button class="btn btn-outline md:w-32" :disabled="exporting" @click="exportPDF">Export PDF</button>
        <button
          v-if="contract?.state === ContractState.reviewed"
          class="btn flex-1 btn-outline"
          :disabled="!isApprover || isSubmitting"
          @click="resubmit"
        >
          Resubmit
        </button>
        <button
          v-if="contract?.state === ContractState.reviewed"
          class="btn flex-1 btn-primary"
          :disabled="!isApprover || isSubmitting"
          @click="reject"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Reject
        </button>
        <button
          v-if="contract?.state === ContractState.reviewed"
          class="btn flex-1 btn-primary"
          :disabled="!isApprover || isSubmitting"
          @click="approve"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Approve
        </button>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
      <ConfirmationModal ref="confirmation-dialog" />
    </div>
  </div>
</template>
