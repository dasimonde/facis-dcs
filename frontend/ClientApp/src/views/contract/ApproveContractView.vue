<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import type { Contract } from '@/models/contract/contract'
import AuditView from '@/modules/contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import TemplatePreview from '@/modules/template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'
import { useTemplateDraftStore } from '@/modules/template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const navStore = useNavStore()
const authStore = useAuthStore()

const { isApprover } = useContractPermissions()

const templateDraftStore = useTemplateDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue } = useSemanticValueVerification()
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

const verificationResult: Ref<VerificationResult | null> = ref(null)

const contract: Ref<Contract | null> = ref(null)
const compareChangesData: Ref<Contract | null> = ref(null)

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
  () => [templateDraftStore.blocks, templateDraftStore.semanticConditions, templateDraftStore.subTemplateSnapshots],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(
          conditionValue,
          templateDraftStore.blocks,
          templateDraftStore.semanticConditions,
          templateDraftStore.subTemplateSnapshots,
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
    })
    if (confirmationResult?.isCanceled) return
    isSubmitting.value = true
    const response = await contractWorkflowService.approve({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    console.error('Failed to approve', err)
  } finally {
    isSubmitting.value = false
  }
}

const resubmit = async () => {
  if (!contract.value) return
  try {
    const confirmationResult = await confirmationDialog.value?.reveal({
      message: 'Add decision note',
      editor: { requiredText: false, placeholder: 'Decision note' },
    })
    if (confirmationResult?.isCanceled) return
    const comment = confirmationResult?.data
    isSubmitting.value = true
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      comments: comment ? [comment] : [],
    })
    if (response.did) {
      await navStore.goToPreviousRoute()
    }
  } catch (err) {
    console.error('Failed to resubmit', err)
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
      console.error('Reason is required for rejection')
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
    console.error('Failed to reject', err)
  } finally {
    isSubmitting.value = false
  }
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
  const cd = preprocessContractData(contractData)
  if (cd) {
    templateDraftStore.reset({
      workflow: 'contract',
      blocks: cd.blocks,
      layout: cd.layout,
      contractData: cd.contractData,
      policies: cd.policies,
      subTemplateSnapshots: cd.subTemplateSnapshots,
    })
    contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  } else {
    templateDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
  }
  verificationResult.value = null
}

const exportPDF = async () => {
  if (contract?.value?.did === null || contract?.value?.did === undefined) {
    return
  }

  const blob = await contractWorkflowService.exportPdf(contract?.value?.did)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `contract-${contract?.value?.did}.pdf`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="!!contract" class="flex flex-1 flex-col">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">Approve Contract</p>
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
                <ContractDetailsEditor
                  :contract="contract"
                  :inserted="{ name: compareChangesData?.name, description: compareChangesData?.description }"
                  disabled
                />
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :layout="templateDraftStore.layout"
                        :blocks="templateDraftStore.blocks"
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
        <button class="btn btn-outline md:w-32" @click="exportPDF">Export PDF</button>
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
          @click="resubmit"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Resubmit
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
