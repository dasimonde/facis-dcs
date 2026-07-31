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
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import ContractVerificationFindingsDialog from '@/components/contract/ContractVerificationFindingsDialog.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract } from '@/models/contract/contract'
import type { UserRole } from '@/types/user-role'
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

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const isAuditingAuthorized = computed(
  () =>
    (['AUDITOR', 'COMPLIANCE_OFFICER', 'SYSTEM_ADMINISTRATOR'] as UserRole[]).some((role) =>
      authStore.user?.roles?.includes(role),
    ) ?? false,
)

const tabs = computed(() => contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft))

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

watch(
  () => !!route.params.did,
  async (value) => {
    if (value) {
      try {
        const id = route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          applyContractDataToDraft(contract.value?.contract_data)
        }
      } catch (err: unknown) {
        reportActionError(err, 'Load contract review')
      }
    }
  },
  { immediate: true },
)

watch(
  () => [dcsDraftStore.blocks, dcsDraftStore.semanticConditions],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(conditionValue, dcsDraftStore.blocks, dcsDraftStore.semanticConditions),
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
  })
  if (response.did) {
    await navStore.goToPreviousRoute()
  }
}

const returnToNegotiation = async () => {
  if (!contract.value) return
  try {
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: 'Comment findings',
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
    })
    if (response.did) {
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
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">Review Contract</p>
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
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor :contract="contract" disabled />
              </div>

              <div v-show="activeTab === 'content'">
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
                      />
                    </div>
                  </div>
                </div>
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
