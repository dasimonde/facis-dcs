<script setup lang="ts">
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import SubmitSelectionDialog from '@/components/SubmitSelectionDialog.vue'
import type { ContractData } from '@/models/contract-data'
import type { Contract } from '@/models/contract/contract'
import type { SelectedUserRole } from '@/models/user'
import AuditView from '@/modules/contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractPlainTextConverter } from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import { buildContractPdfArchive } from '@/modules/contract-workflow-engine/utils/buildContractPdfArchive'
import { toPdfData } from '@/modules/contract-workflow-engine/utils/contractPdfConverter'
import { downloadContractPdf } from '@/modules/contract-workflow-engine/utils/contractPdfExporter'
import TemplatePreview from '@/modules/template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useTemplateDraftStore } from '@/modules/template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useErrorStore } from '@/stores/error-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const navStore = useNavStore()

const authStore = useAuthStore()
const templateDraftStore = useTemplateDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const contractContentValuesStore = useContractContentValuesStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { convertContractToPlainTextBlocks } = useContractPlainTextConverter()
const { activeTab } = storeToRefs(contractEditorUiStore)

const errorStore = useErrorStore()

const contract: Ref<Contract | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)

const isCreator = computed(() => {
  return contract.value?.created_by === authStore.user?.username
})

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const isAuditingAuthorized = computed(
  () =>
    (['AUDITOR', 'COMPLIANCE_OFFICER', 'SYSTEM_ADMINISTRATOR'] as UserRole[]).some((role) =>
      authStore.user?.roles?.includes(role),
    ) ?? false,
)

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft).filter((tab) => {
    // Don't show diff tab in the contract view.
    return tab.id !== 'diff'
  }),
)

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
        console.error('Failed to load contract', err)
      }
    }
  },
  { immediate: true },
)

watch(
  () => [
    templateDraftStore.documentBlocks,
    templateDraftStore.semanticConditions,
    templateDraftStore.subTemplateSnapshots,
  ],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(
          conditionValue,
          templateDraftStore.documentBlocks,
          templateDraftStore.semanticConditions,
          templateDraftStore.subTemplateSnapshots,
        ),
    )
    contractContentValuesStore.removeSemanticConditionValues(invalidValues)
  },
  { deep: true },
)

const submitContract = async (result: SelectedUserRole[]) => {
  if (!contract.value) return
  const isSemanticValueValid = verifySemanticValues()
  if (!isSemanticValueValid) return
  try {
    const reviewers = result.filter((user) => user.role === 'CONTRACT_REVIEWER').map((user) => user.user.username)
    const approvers = result.filter((user) => user.role === 'CONTRACT_APPROVER').map((user) => user.user.username)
    const negotiators = result.filter((user) => user.role === 'CONTRACT_NEGOTIATOR').map((user) => user.user.username)
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      reviewers,
      approvers,
      negotiators,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    console.error('Contract Submission failed', error)
  }
}

const submitRejectedTemplate = async () => {
  if (!contract.value) return
  const isSemanticValueValid = verifySemanticValues()
  if (!isSemanticValueValid) return
  try {
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (error) {
    console.error('Contract Submission failed', error)
  }
}

const verifySemanticValues = (): boolean => {
  const subTemplateSemanticConditions = templateDraftStore?.subTemplateSnapshots?.map((subTemplate) => {
    return {
      templateId: subTemplate.did,
      version: subTemplate.version,
      document_number: subTemplate.document_number,
      semanticConditions: subTemplate.template_data?.semanticConditions ?? [],
    }
  })
  const result = verifySemanticValue(
    templateDraftStore.semanticConditions,
    subTemplateSemanticConditions,
    contractContentValuesStore.semanticConditionValues,
    templateDraftStore.documentBlocks,
  )
  verificationResult.value = result
  if (result.isValid) {
    return true
  } else {
    result.errors.forEach((error) => errorStore.add(error.message))
  }
  // go to content tab and highlight semantic inconsistencies
  contractEditorUiStore.setActiveTab('content')
  return false
}

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract', isTemplateEditable: false })
})

onUnmounted(() => {
  templateDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
  verificationResult.value = null
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    templateDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    verificationResult.value = null
    return
  }
  const cd = preprocessContractData(contractData as ContractData)
  templateDraftStore.reset({
    workflow: 'contract',
    documentOutline: cd.documentOutline ?? [],
    documentBlocks: cd.documentBlocks ?? [],
    semanticConditions: cd.semanticConditions ?? [],
    subTemplateSnapshots: cd.subTemplateSnapshots ?? [],
    templateDataVersion: cd.templateDataVersion,
  })
  contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  verificationResult.value = null
}

const exportPdf = async () => {
  const id = route.params.did
  if (!id || Array.isArray(id)) return
  const contract = await contractWorkflowService.retrieveById({ did: id })
  if (!contract) return
  const blocks = convertContractToPlainTextBlocks(contract.contract_data)
  const pdfData = toPdfData(blocks)
  const archive = await buildContractPdfArchive(contract)
  const title = `${contract.name ?? 'contract'}`
  const filename = `${title}.pdf`
  downloadContractPdf(pdfData, filename, title, { displayTitleInContent: true, archive })
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div v-if="!!contract">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">View Contract</p>
            <div role="tablist" class="tabs-border tabs tabs-lg">
              <a
                v-for="tab in tabs"
                :key="tab.id"
                role="tab"
                class="tab"
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
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor :contract="contract" disabled />
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :document-outline="templateDraftStore.documentOutline"
                        :document-blocks="templateDraftStore.documentBlocks"
                        :semantic-conditions="templateDraftStore.semanticConditions"
                        :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                        :verification-result="verificationResult"
                        :sub-template-snapshots="templateDraftStore.subTemplateSnapshots"
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
        <button class="btn btn-outline md:w-32" @click="exportPdf">Export PDF</button>
        <template v-if="isCreator">
          <SubmitSelectionDialog
            v-if="contract?.state === ContractState.draft"
            dialog-type="contract"
            class="btn flex-1 btn-primary"
            @submit="submitContract"
          />
          <button
            v-else-if="contract?.state === ContractState.rejected"
            class="btn flex-1 btn-primary"
            @click="submitRejectedTemplate"
          >
            Submit
          </button>
        </template>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
    </div>
  </div>
</template>
