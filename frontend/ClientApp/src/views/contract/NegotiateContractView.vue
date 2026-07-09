<script setup lang="ts">
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import NegotiationList from '@/components/lists/contract/negotiation/NegotiationList.vue'
import { useScrollStore } from '@/core/store/scroll'
import type { ContractData, SemanticConditionValue } from '@/models/contract-data'
import type { Contract, ContractChangeRequest } from '@/models/contract/contract'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'
import AuditView from '@/modules/contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import ContractHistoryDiffView from '@/modules/contract-workflow-engine/components/ContractHistoryDiffView.vue'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import { useSemanticValueVerification } from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import TemplatePreview from '@/modules/template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'
import { useTemplateDraftStore } from '@/modules/template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import {
  buildContractDocument,
  getSemanticConditionsFromTemplateData,
} from '@/modules/template-repository/store/dcsDraftStore'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useErrorStore } from '@/stores/error-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import type { UserRole } from '@/types/user-role'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, ref, useTemplateRef, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const navStore = useNavStore()

const authStore = useAuthStore()
const issuer = computed(() => authStore.user?.issuer)

const errorStore = useErrorStore()

const templateDraftStore = useTemplateDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)
const contractContentValuesStore = useContractContentValuesStore()
const scrollStore = useScrollStore()

const isSubmitting = ref(false)

const { isCreator, isReviewer } = useContractPermissions()

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

const verificationResult = computed(() => {
  const subTemplateSemanticConditions = templateDraftStore.subTemplateSnapshots.map((subTemplate) => ({
    templateId: subTemplate.did,
    version: subTemplate.version,
    document_number: subTemplate.document_number,
    semanticConditions: getSemanticConditionsFromTemplateData(subTemplate.template_data),
  }))
  return verifySemanticValue(
    templateDraftStore.semanticConditions,
    subTemplateSemanticConditions,
    contractContentValuesStore.semanticConditionValues,
    templateDraftStore.blocks,
  )
})

const contract: Ref<Contract | null> = ref(null)
const editedContract: Ref<Contract | null> = ref(null)
const compareChangesData: Ref<(Contract & { exp_notice_period_str: string; exp_policy_str: string }) | null> = ref(null)

const hasChangeRequest = computed(() => {
  return (
    changedName.value ||
    changedDescription.value ||
    changedContractData.value ||
    changeExpNoticePeriod.value ||
    changeExpPolicy.value
  )
})

const changedName = computed(() => editedContract.value?.name !== contract.value?.name)
const changedDescription = computed(() => editedContract.value?.description !== contract.value?.description)
const changeExpNoticePeriod = computed(
  () => editedContract.value?.exp_notice_period != contract.value?.exp_notice_period,
)
const changeExpPolicy = computed(() => editedContract.value?.exp_policy != contract.value?.exp_policy)
const changedContractData = computed(() => {
  const storedValues = contractContentValuesStore.semanticConditionValues
  const contractValues = contract.value?.contract_data?.semanticConditionValues ?? []

  return !semanticConditionValuesEqual(storedValues, contractValues)
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
    documentId: contract.value.did,
    name: editedContract.value?.name ?? contract.value.name,
    description: editedContract.value?.description ?? contract.value.description,
    blocks: templateDraftStore.blocks,
    layout: templateDraftStore.layout,
    contractData: templateDraftStore.contractData,
    policies: templateDraftStore.policies,
    subTemplateSnapshots: templateDraftStore.subTemplateSnapshots,
    semanticConditionValues: contractContentValuesStore.semanticConditionValues,
    sourceTemplate: contract.value.contract_data?.sourceTemplate,
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
    }
  } catch (err: unknown) {
    console.error('Failed to load contract', err)
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

const negotiateContractChange = async () => {
  if (!contract.value || !editedContract.value || !issuer.value) return
  isSubmitting.value = true
  try {
    const changeRequest: ContractChangeRequest = {}
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
    const response = await contractWorkflowService.negotiate({
      did: contract.value?.did,
      updated_at: contract.value?.updated_at,
      negotiated_by: issuer.value,
      change_request: changeRequest,
    })
    if (response.did) {
      await loadContract()
    }
  } catch (err) {
    console.error('Failed to submit change request', err)
  } finally {
    isSubmitting.value = false
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
    console.error('Failed to submit', err)
  } finally {
    isSubmitting.value = false
  }
}

const hasOpenDecisions = computed(
  () =>
    contract.value?.negotiations?.some((negotiation) =>
      negotiation.negotiation_decisions.some((decision) => !decision.decision),
    ) ?? false,
)

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract' })
})

onUnmounted(() => {
  templateDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    templateDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
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
}

const templatePreviewContent = useTemplateRef<HTMLElement>('template-preview-content')

const originalSemanticConditionValues: Ref<SemanticConditionValue[]> = ref([])
const originalValuesWereCached = ref(false)

const handleSelectedNegotiation = async (negotiation: ContractNegotiation | null) => {
  if (!contract.value) return
  compareChangesData.value = !!negotiation
    ? {
        ...contract.value,
        name: negotiation.change_request.name
          ? `${contract.value.name} -> ${negotiation.change_request.name}`
          : contract.value.name,
        exp_notice_period_str: negotiation.change_request.exp_notice_period
          ? `${contract.value.exp_notice_period} -> ${negotiation.change_request.exp_notice_period}`
          : (contract.value.exp_notice_period?.toString() ?? ''),
        exp_policy_str: negotiation.change_request.exp_policy
          ? `${contract.value.exp_policy} -> ${negotiation.change_request.exp_policy}`
          : (contract.value.exp_policy ?? ''),
        description: negotiation.change_request.description
          ? `${contract.value.description} -> ${negotiation.change_request.description}`
          : contract.value.description,
        contract_data: contract.value.contract_data,
      }
    : null

  if (compareChangesData.value && negotiation) {
    if (!originalValuesWereCached.value) {
      originalSemanticConditionValues.value = [...contractContentValuesStore.semanticConditionValues]
      originalValuesWereCached.value = true
    }
    const negotiationValues = negotiation.change_request.contract_data?.semanticConditionValues ?? []

    const originalValuesMap = new Map(
      contract.value.contract_data?.semanticConditionValues?.map((value) => [
        `${value.blockId}|${value.conditionId}|${value.parameterName}`,
        String(value.parameterValue),
      ]) ?? [],
    )
    const negotiationValuesMap = new Map(
      negotiationValues.map((value) => [
        `${value.blockId}|${value.conditionId}|${value.parameterName}`,
        String(value.parameterValue),
      ]),
    )
    negotiationValues.forEach((value) => contractContentValuesStore.setSemanticConditionValue(value))

    await nextTick()

    requestAnimationFrame(() => {
      const inputs = Array.from(templatePreviewContent.value?.querySelectorAll('input') ?? [])

      const highlightedValues = new Set<string>()
      for (const [key, negotiationValue] of negotiationValuesMap.entries()) {
        const originalValue = originalValuesMap.get(key)
        if (negotiationValue !== originalValue) {
          highlightedValues.add(negotiationValue)
        }
      }

      inputs.forEach((input) => {
        if (highlightedValues.has(input.value)) {
          input.classList.add('!border-warning', '!border-2')
          input.style.setProperty('border-color', '#f59e0b', 'important')
          input.style.setProperty('border-width', '2px', 'important')
        } else {
          input.classList.remove('!border-warning', '!border-2')
          input.style.removeProperty('border-color')
          input.style.removeProperty('border-width')
        }
      })
    })

    scrollStore.scrollToTop()
  } else {
    contractContentValuesStore.reset({ semanticConditionValues: originalSemanticConditionValues.value })
    originalValuesWereCached.value = false
    requestAnimationFrame(() => {
      const inputs = Array.from(templatePreviewContent.value?.querySelectorAll('input') ?? [])
      inputs.forEach((input) => {
        input.classList.remove('!border-warning', '!border-2')
        input.style.removeProperty('border-color')
        input.style.removeProperty('border-width')
      })
    })
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

const hasActiveNegotiations = computed(() => {
  return (
    contract.value?.negotiations?.some(
      (negotiation) => negotiation.contract_version === contract.value?.contract_version,
    ) ?? false
  )
})

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
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div v-if="!!contract && !!editedContract && !!shownData">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">Negotiate Contract</p>
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
                  :contract="shownData"
                  :inserted="{
                    name: compareChangesData?.name,
                    description: compareChangesData?.description,
                    exp_notice_period: compareChangesData?.exp_notice_period_str,
                    exp_policy: compareChangesData?.exp_policy_str,
                  }"
                />
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div ref="template-preview-content">
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
            </div>
          </div>
        </div>
      </div>
      <template v-if="activeTab !== 'audit' && hasActiveNegotiations">
        <div class="divider"></div>
        <div class="mx-auto max-w-4xl p-6">
          <div class="text-lg">Active negotiations</div>
          <NegotiationList :contract="contract" @selected-negotiation="handleSelectedNegotiation" />
        </div>
      </template>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <button class="btn btn-outline md:w-32" @click="exportPDF">Export PDF</button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting || !hasChangeRequest || !!compareChangesData"
          @click="negotiateContractChange"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Change Proposal
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn flex-1 btn-primary"
          :disabled="
            (!isCreator && !isReviewer) || isSubmitting || hasChangeRequest || hasOpenDecisions || !!compareChangesData
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
