<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, type Ref, ref, useId, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import WorkflowStageBanner from '@core/components/WorkflowStageBanner.vue'
import { useScrollStore } from '@core/store/scroll'
import { contractStory, toBannerActions } from '@core/workflow-story'
import AddBlockModal from '@template-repository/components/builder-editor/AddBlockModal.vue'
import BuilderPreviewDialog from '@template-repository/components/builder-editor/BuilderPreviewDialog.vue'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import BuilderEditor from '@template-repository/components/BuilderEditor.vue'
import ClausesEditor from '@template-repository/components/ClausesEditor.vue'
import DataObjectsEditor from '@template-repository/components/data-objects/DataObjectsEditor.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { buildContractDocument } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import ViewContractTemplateView from '@template-repository/views/ViewContractTemplateView.vue'
import ContractDetailsEditor from '@contract-workflow-engine/components/ContractDetailsEditor.vue'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ParticipantSelectionDialog from '@/components/ParticipantSelectionDialog.vue'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useContractsStore } from '@/stores/contracts-store'
import { useErrorStore } from '@/stores/error-store'
import { ContractState } from '@/types/contract-state'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract } from '@/models/contract/contract'
import type { ContractData } from '@/models/contract/contract-data'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'
import type { ParticipantSelection } from '@/utils/participant-selection'
import type { SemanticConditionValueSetter } from '@contract-workflow-engine/models/contract-content-values-store'

const route = useRoute()
const router = useRouter()

const errorStore = useErrorStore()
const contractStore = useContractsStore()

const { hasApprovedTemplates, approvedTemplates } = storeToRefs(contractStore)
const dcsDraftStore = useDcsDraftStore()
const contractContentValuesStore = useContractContentValuesStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)

const did = ref<string | null>(null)
const isEditMode = computed(() => !!route.params.did || !!did.value)
const isSubmitting = ref(false)
const selectedTemplate: Ref<PartialContractTemplate | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)
const detailsValidationAttempted = ref(false)
const nameError = computed(() =>
  detailsValidationAttempted.value && !contract.value?.name?.trim() ? 'Global Name is required.' : null,
)
const descriptionError = computed(() =>
  detailsValidationAttempted.value && !contract.value?.description?.trim() ? 'Base Description is required.' : null,
)
const selectedParentContractDid = ref<string | null>(null)

const templatePickerId = useId()
const viewTemplatePickerLabelId = useId()
const parentContractPickerLabelId = useId()

const contract: Ref<Contract | null> = ref(null)

const draftContracts = computed(() => contractStore.contracts.filter((c) => c.state === ContractState.draft))

const canSubmit = computed(() => isEditMode.value || (hasApprovedTemplates.value && selectedTemplate.value !== null))
const canSubmitContract = computed(
  () =>
    isEditMode.value &&
    contract.value !== null &&
    (contract.value.state === ContractState.draft || contract.value.state === ContractState.rejected),
)

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  if (!isEditMode.value) return null
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft).filter((tab) => tab.id !== 'audit'),
)

const story = computed(() =>
  contractStory(contract.value?.state, { extrinsicLifecycle: contract.value?.extrinsic_lifecycle }),
)

function buildCurrentContractData(): ContractData | undefined {
  if (!contract.value) return undefined
  return buildContractDocument({
    documentId:
      ((contract.value.contract_data as Record<string, unknown> | undefined)?.['@id'] as string | undefined) ??
      contract.value.did,
    storedDocument: contract.value.contract_data,
    name: contract.value.name,
    description: contract.value.description,
    blocks: dcsDraftStore.blocks,
    layout: dcsDraftStore.layout,
    contractFields: dcsDraftStore.contractFields,
    contractData: dcsDraftStore.contractData,
    policies: dcsDraftStore.policies,
    semanticConditionValues: contractContentValuesStore.semanticConditionValues,
    parentContractDid: selectedParentContractDid.value ?? undefined,
    derivedFromTemplate: contract.value.contract_data?.derivedFromTemplate,
  })
}

function currentExpNoticePeriod(): number | undefined {
  const value = contract.value?.exp_notice_period as unknown
  if (value === undefined || value === null || value === '') return undefined
  const numericValue = Number(value)
  return Number.isFinite(numericValue) ? numericValue : undefined
}

async function saveContractDraftForSubmit(): Promise<Contract> {
  if (!contract.value) throw new Error('No contract selected')

  await contractWorkflowService.update({
    did: contract.value.did,
    updated_at: contract.value.updated_at,
    exp_notice_period: currentExpNoticePeriod(),
    exp_policy: contract.value.exp_policy,
    name: contract.value.name,
    description: contract.value.description,
    contract_data: buildCurrentContractData(),
  })

  const updatedContract = await contractWorkflowService.retrieveById({ did: contract.value.did })
  if (!updatedContract) throw new Error('Could not reload contract after update')
  contract.value = updatedContract
  return updatedContract
}

function verifySemanticValues(): boolean {
  const result = verifySemanticValue(
    dcsDraftStore.semanticConditions,
    contractContentValuesStore.semanticConditionValues,
    dcsDraftStore.blocks,
  )
  verificationResult.value = result
  if (result.isValid) {
    return true
  }
  result.errors.forEach((error) => errorStore.add(error.message))
  contractEditorUiStore.setActiveTab('content')
  return false
}

async function verifyContractDetails(): Promise<boolean> {
  detailsValidationAttempted.value = true
  if (!nameError.value && !descriptionError.value) return true
  contractEditorUiStore.setActiveTab('details')
  await nextTick()
  const testId = nameError.value ? 'contract-global-name' : 'contract-base-description'
  document.querySelector<HTMLElement>(`[data-testid="${testId}"]`)?.focus()
  return false
}

const createContract = async ({ counterparty }: ParticipantSelection) => {
  isSubmitting.value = true
  try {
    if (selectedTemplate.value) {
      const response = await contractWorkflowService.create({
        template_did: selectedTemplate.value.did,
        counterparty,
      })
      did.value = response.did
      if (selectedParentContractDid.value) {
        const newContract = await contractWorkflowService.retrieveById({ did: response.did })
        if (!newContract?.contract_data) {
          throw new Error('Could not reload created contract')
        }
        const contractData = {
          ...newContract.contract_data,
          'dcs:parentContract': { '@id': selectedParentContractDid.value },
        } as ContractData
        await contractWorkflowService.update({
          did: newContract.did,
          updated_at: newContract.updated_at,
          contract_data: contractData,
        })
      }
      errorStore.add('Contract created.', 'info')
    }
  } catch (error) {
    reportActionError(error, 'Create contract')
  } finally {
    isSubmitting.value = false
  }
}

const updateContract = async () => {
  if (!(await verifyContractDetails())) return
  isSubmitting.value = true
  try {
    if (contract.value) {
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        exp_notice_period: currentExpNoticePeriod(),
        exp_policy: contract.value.exp_policy,
        name: contract.value.name,
        description: contract.value.description,
        contract_data: buildCurrentContractData(),
      })
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    reportActionError(error, 'Update contract')
  } finally {
    isSubmitting.value = false
  }
}

const submitContract = async () => {
  if (!contract.value || !(await verifyContractDetails()) || !verifySemanticValues()) return
  isSubmitting.value = true
  try {
    const updatedContract = await saveContractDraftForSubmit()
    const response = await contractWorkflowService.submit({
      did: updatedContract.did,
      updated_at: updatedContract.updated_at,
    })
    if (response.did) {
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    reportActionError(error, 'Submit contract')
  } finally {
    isSubmitting.value = false
  }
}

const submitRejectedContract = async () => {
  if (!contract.value || !(await verifyContractDetails()) || !verifySemanticValues()) return
  isSubmitting.value = true
  try {
    const updatedContract = await saveContractDraftForSubmit()
    const response = await contractWorkflowService.submit({
      did: updatedContract.did,
      updated_at: updatedContract.updated_at,
    })
    if (response.did) {
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    reportActionError(error, 'Resubmit contract')
  } finally {
    isSubmitting.value = false
  }
}

watch(
  isEditMode,
  async (value) => {
    if (value) {
      try {
        const id = did.value ?? route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          applyContractDataToDraft(contract.value?.contract_data)
          selectedParentContractDid.value = contract.value?.contract_data?.['dcs:parentContract']?.['@id'] ?? null
          const uneditableStates = [ContractState.approved, ContractState.terminated].map((s) => s.toLowerCase())
          templateEditorUiStore.setTemplateEditable(
            !uneditableStates.includes(contract.value?.state.toLowerCase() ?? ''),
          )
        }
      } catch (err: unknown) {
        reportActionError(err, 'Load contract')
      }
    } else {
      await contractStore.loadApprovedTemplates()
    }
  },
  { immediate: true },
)

onMounted(async () => {
  if (!contractStore.hasContracts) {
    await contractStore.loadContracts()
  }
})

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

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract' })
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

const scrollStore = useScrollStore()

watch(selectedTemplate, (value) => {
  if (!!value?.did) {
    scrollStore.addGutter()
  } else {
    scrollStore.removeGutter()
  }
})

onBeforeRouteLeave(() => {
  scrollStore.removeGutter()
})
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="!isEditMode" class="flex flex-1 flex-col">
      <div v-if="!selectedTemplate" class="flex flex-1 items-center justify-center px-6 py-20">
        <label :for="templatePickerId" class="sr-only">Pick a template</label>
        <select
          :id="templatePickerId"
          v-model="selectedTemplate"
          class="select w-150"
          :disabled="!hasApprovedTemplates"
        >
          <option :value="null" disabled selected>
            {{ hasApprovedTemplates ? 'Pick a template' : 'No templates available' }}
          </option>
          <option v-for="template in approvedTemplates" :key="template.did" :value="template">
            Version {{ template.version }} - {{ template.name?.slice(0, 80)
            }}{{ (template.name?.length ?? 0) > 80 ? '…' : '' }}
          </option>
        </select>
      </div>
      <ViewContractTemplateView v-else :did="selectedTemplate.did" :embedded="true">
        <template #before-tabs>
          <div class="flex items-end gap-4">
            <div class="flex-1">
              <p :id="viewTemplatePickerLabelId" class="mb-1 text-xs font-semibold text-base-content/70">Template</p>
              <select
                v-model="selectedTemplate"
                :aria-labelledby="viewTemplatePickerLabelId"
                class="select w-full select-sm"
              >
                <option v-for="template in approvedTemplates" :key="template.did" :value="template">
                  Version {{ template.version }} - {{ template.name?.slice(0, 80)
                  }}{{ (template.name?.length ?? 0) > 80 ? '…' : '' }}
                </option>
              </select>
            </div>
            <div v-if="draftContracts.length > 0" class="flex-1">
              <p :id="parentContractPickerLabelId" class="mb-1 text-xs font-semibold text-base-content/70">
                Add to existing contract (optional)
              </p>
              <select
                v-model="selectedParentContractDid"
                :aria-labelledby="parentContractPickerLabelId"
                class="select w-full select-sm"
              >
                <option :value="null">— none —</option>
                <option v-for="c in draftContracts" :key="c.did" :value="c.did">
                  {{ c.name ?? c.did }}
                </option>
              </select>
            </div>
          </div>
        </template>
      </ViewContractTemplateView>
    </div>
    <div v-else-if="!!contract" class="flex flex-1 flex-col">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">
              {{ isEditMode ? 'Update Contract' : 'Create Contract' }}
            </p>
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
                <ContractDetailsEditor
                  :contract="contract"
                  :name-error="nameError ?? undefined"
                  :description-error="descriptionError ?? undefined"
                />
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
              <!-- CLAUSES TAB -->
              <div v-show="activeTab === 'clauses'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <ClausesEditor />
                  </div>
                </div>
              </div>

              <!-- BUILDER TAB -->
              <div v-show="activeTab === 'builder'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body">
                    <div class="mb-2 flex items-center justify-between">
                      <h2 class="card-title text-sm">Builder</h2>
                      <button
                        type="button"
                        class="btn btn-sm btn-secondary"
                        @click="templateEditorUiStore.togglePreviewDialog"
                      >
                        Preview
                      </button>
                    </div>
                    <BuilderEditor />
                  </div>
                </div>
                <AddBlockModal />
                <BuilderPreviewDialog />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <ParticipantSelectionDialog
          v-if="!isEditMode"
          :disabled="isSubmitting || !canSubmit"
          class="btn flex-1 btn-primary"
          @submit="createContract"
        />
        <button
          v-if="isEditMode"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting || !canSubmit"
          @click="updateContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Update
        </button>
        <button
          v-if="contract?.state === ContractState.draft && canSubmitContract"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting"
          @click="submitContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Submit
        </button>
        <button
          v-else-if="contract?.state === ContractState.rejected && canSubmitContract"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting"
          @click="submitRejectedContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Submit
        </button>
      </div>
    </div>
  </div>
</template>
