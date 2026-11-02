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
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import ContractVerificationFindingsDialog from '@/components/contract/ContractVerificationFindingsDialog.vue'
import PeerProposalPendingAlert from '@/components/contract/PeerProposalPendingAlert.vue'
import ReviewBlockAnnotationsPanel from '@/components/contract/ReviewBlockAnnotationsPanel.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { resolveLocalInstanceDid } from '@/services/did-service'
import { useAuthStore } from '@/stores/auth-store'
import { useNavStore } from '@/stores/nav-store'
import { contractAuditRoles } from '@/types/contract-audit-roles'
import { ContractState } from '@/types/contract-state'
import { hasPendingPeerProposalAfterSettle } from '@/utils/contract-negotiations'
import { reportActionError } from '@/utils/report-action-error'
import {
  annotationCount,
  draftToAnnotations,
  incompleteAnnotationCount,
  type ReviewAnnotationDraft,
} from '@/utils/review-block-annotations'
import type { Contract } from '@/models/contract/contract'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

const route = useRoute()
const navStore = useNavStore()
const authStore = useAuthStore()

const { isReviewer } = useContractPermissions()

const dcsDraftStore = useDcsDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)
const contractContentValuesStore = useContractContentValuesStore()

const confirmationDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-dialog')

const isSubmitting = ref(false)
/** Frontend-only drafts until Reject/Approve submit. */
const annotationDraft = ref<ReviewAnnotationDraft>({})

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
    'audit',
  ]),
)

const story = computed(() =>
  contractStory(contract.value?.state, { extrinsicLifecycle: contract.value?.extrinsic_lifecycle }),
)

const verificationResult = computed(() => {
  try {
    return verifySemanticValue(
      dcsDraftStore.semanticConditions,
      contractContentValuesStore.semanticConditionValues,
      dcsDraftStore.blocks,
    )
  } catch {
    return { isValid: false, errors: [] }
  }
})

const contract: Ref<Contract | null> = ref(null)
const localInstanceDid = ref('')

const showPeerProposalAlert = computed(() => hasPendingPeerProposalAfterSettle(contract.value, localInstanceDid.value))

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
          // The audit trail records that the reviewer opened the submitted contract (DCS-IR-CWE-04).
          if (isReviewer.value && loaded?.state === ContractState.submitted) {
            void contractWorkflowService.review({ did: id }).catch(() => undefined)
          }
        }
      } catch (err: unknown) {
        reportActionError(err, 'Load contract review')
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

const runLocalSemanticVerification = () => {
  return verifySemanticValue(
    dcsDraftStore.semanticConditions,
    contractContentValuesStore.semanticConditionValues,
    dcsDraftStore.blocks,
  )
}

const forwardToApproval = async (comment: string) => {
  const currentContract = contract.value
  if (!isReviewer.value || currentContract?.state !== ContractState.submitted || !currentContract.updated_at) {
    reportActionError(
      new Error('Only a reviewer can forward a current submitted contract to approval.'),
      'Forward contract',
    )
    return
  }
  const response = await contractWorkflowService.submit({
    did: currentContract.did,
    updated_at: currentContract.updated_at,
    forward_to: 'APPROVAL',
    comments: comment ? [comment] : [],
    annotations: draftToAnnotations(annotationDraft.value),
  })
  if (response.did) {
    annotationDraft.value = {}
    await navStore.goToPreviousRoute()
  }
}

const returnToNegotiation = async () => {
  if (!contract.value) return
  try {
    const ready = annotationCount(annotationDraft.value)
    const incomplete = incompleteAnnotationCount(annotationDraft.value)
    if (incomplete > 0) {
      contractEditorUiStore.setActiveTab('content')
      reportActionError(
        new Error(
          `${incomplete} annotation(s) still need text. Fill them in under Annotations (Contract Content), or remove them.`,
        ),
        'Return contract to negotiation',
      )
      return
    }
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: ready > 0 ? `Return to negotiation with ${ready} annotation(s).` : 'Comment findings',
      editor: { requiredText: false, placeholder: 'Comments, findings...' },
    })
    if (confirmationResult?.isCanceled) return
    const comment = confirmationResult?.data
    isSubmitting.value = true
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      forward_to: 'REJECT',
      comments: comment ? [comment] : [],
      annotations: draftToAnnotations(annotationDraft.value),
    })
    if (response.did) {
      annotationDraft.value = {}
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    reportActionError(err, 'Return contract to negotiation')
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
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
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
              <span>Review Contract</span>
              <span v-if="isReviewer" class="font-normal normal-case">· as Contract Reviewer</span>
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
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor :contract="contract" disabled />
              </div>

              <div v-show="activeTab === 'content'" class="space-y-6">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body">
                    <TemplatePreview
                      :layout="dcsDraftStore.layout"
                      :blocks="dcsDraftStore.blocks"
                      :semantic-conditions="dcsDraftStore.semanticConditions"
                      :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                      :verification-result="verificationResult"
                      :set-semantic-condition-value="setSemanticConditionValue"
                      :highlighted-block-ids="Object.keys(annotationDraft)"
                      disabled
                    />
                  </div>
                </div>
                <div
                  v-if="contract?.state === ContractState.submitted && isReviewer"
                  class="card border border-base-300 bg-base-100 shadow-sm"
                >
                  <div class="card-body">
                    <ReviewBlockAnnotationsPanel
                      v-model="annotationDraft"
                      :blocks="dcsDraftStore.blocks"
                      :layout="dcsDraftStore.layout"
                      :disabled="isSubmitting"
                    />
                  </div>
                </div>
              </div>

              <div v-show="activeTab === 'diff'">
                <ContractHistoryDiffView
                  v-if="contract"
                  :contract-did="contract.did"
                  :contract-state="contract.state"
                  :current-contract-data="contract.contract_data"
                  :current-details="{
                    name: contract.name,
                    description: contract.description,
                    contract_version: contract.contract_version,
                    start_date: contract.start_date,
                    exp_date: contract.exp_date,
                    exp_policy: contract.exp_policy,
                    exp_notice_period: contract.exp_notice_period,
                  }"
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
          v-if="contract?.state === ContractState.submitted"
          class="btn flex-1 btn-primary"
          :disabled="!isReviewer || isSubmitting"
          @click="returnToNegotiation"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Reject
        </button>
        <ContractVerificationFindingsDialog
          v-if="contract?.state === ContractState.submitted"
          class="btn flex-1 btn-primary"
          :disabled="!isReviewer || isSubmitting"
          :verify="runLocalSemanticVerification"
          :submit="forwardToApproval"
        />
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
      <ConfirmationModal ref="confirmation-dialog" />
    </div>
  </div>
</template>
