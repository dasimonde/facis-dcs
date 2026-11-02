<script setup lang="ts">
import { isAxiosError } from 'axios'
import { storeToRefs } from 'pinia'
import { computed, nextTick, onMounted, onUnmounted, type Ref, ref, useId, useTemplateRef, watch } from 'vue'
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
import TemplateVerificationPanel from '@template-repository/components/TemplateVerificationPanel.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { buildContractDocument } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { CONTRACT_PARTY_ROLE_OPTIONS } from '@template-repository/utils/ontology-domain-fields'
import ViewContractTemplateView from '@template-repository/views/ViewContractTemplateView.vue'
import ContractDetailsEditor from '@contract-workflow-engine/components/ContractDetailsEditor.vue'
import ContractEvidencePanel from '@contract-workflow-engine/components/ContractEvidencePanel.vue'
import ContractHistoryDiffView from '@contract-workflow-engine/components/ContractHistoryDiffView.vue'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import ParticipantSelectionDialog from '@/components/ParticipantSelectionDialog.vue'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useContractsStore } from '@/stores/contracts-store'
import { useErrorStore } from '@/stores/error-store'
import { ContractState, isEditableContractState } from '@/types/contract-state'
import { declaredPartyRoles, type ParticipantSelection } from '@/utils/participant-selection'
import { reportActionError } from '@/utils/report-action-error'
import type { Contract } from '@/models/contract/contract'
import type { ContractData } from '@/models/contract/contract-data'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'
import type { ContractHistoryItem } from '@/models/responses/contract-response'
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
const showTemplateCheck = ref(false)
const templateLoadState = ref<'loading' | 'loaded' | 'error'>('loading')
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
  () => isEditMode.value && contract.value !== null && isEditableContractState(contract.value.state),
)

const isContractMutable = computed(() => !isEditMode.value || isEditableContractState(contract.value?.state))

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  if (!isEditMode.value || !isContractMutable.value) return null
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(
    contract.value?.state ?? ContractState.draft,
    isEditMode.value ? ['details', 'content', 'diff'] : ['details', 'content'],
  ),
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
    startDate: contract.value.start_date,
    expirationDate: contract.value.exp_date,
    expirationNoticePeriod: currentExpNoticePeriod(),
    expirationPolicy: contract.value.exp_policy,
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

function isUpdatedElsewhereConflict(error: unknown): boolean {
  if (!isAxiosError(error)) return false
  if (error.response?.status === 409) return true
  const message = error.response?.data?.message
  return typeof message === 'string' && message.toLowerCase().includes('updated elsewhere')
}

async function saveContractDraftForSubmit(): Promise<Contract> {
  if (!contract.value) throw new Error('No contract selected')

  // Create / parent-link / async PDF can advance updated_at before Submit's
  // save-Update; one reload+retry matches the server 409 temporary conflict.
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        start_date: contract.value.start_date,
        exp_date: contract.value.exp_date,
        exp_notice_period: currentExpNoticePeriod(),
        exp_policy: contract.value.exp_policy,
        name: contract.value.name,
        description: contract.value.description,
        contract_data: buildCurrentContractData(),
      })
      break
    } catch (error) {
      if (!isUpdatedElsewhereConflict(error) || attempt === 1) throw error
      const refreshed = await contractWorkflowService.retrieveById({ did: contract.value.did })
      if (!refreshed) throw error
      contract.value = refreshed
    }
  }

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

const createContract = async ({ counterparty, originatorRole, parties }: ParticipantSelection) => {
  if (
    templateRoleState.value !== 'ready' ||
    !originatorRole ||
    !templatePartyRoles.value.some((role) => role.value === originatorRole)
  ) {
    reportActionError(
      new Error('Select one of the two catalogued roles declared by the loaded template.'),
      'Create contract',
    )
    return
  }
  isSubmitting.value = true
  try {
    if (selectedTemplate.value) {
      const response = await contractWorkflowService.create({
        template_did: selectedTemplate.value.did,
        counterparty,
        originator_role: originatorRole,
        parties,
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
  if (!isContractMutable.value) return
  if (!(await verifyContractDetails())) return
  isSubmitting.value = true
  try {
    if (contract.value) {
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        start_date: contract.value.start_date,
        exp_date: contract.value.exp_date,
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

const confirmationDialog = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-dialog')

// The editor state as last loaded or saved; a different snapshot means unsaved edits.
const savedSnapshot = ref('')
function editorSnapshot(): string {
  const current = contract.value
  return JSON.stringify([
    current?.name,
    current?.description,
    current?.start_date,
    current?.exp_date,
    current?.exp_policy,
    currentExpNoticePeriod(),
    buildCurrentContractData(),
  ])
}

// Rollback (DCS-FR-CWE-08): a saved version becomes the content of a new
// version; the version it replaces stays in the history. Every versioned field
// comes back, and unsaved edits are discarded only after confirmation.
const restoreVersion = async (item: ContractHistoryItem) => {
  if (!contract.value || !isContractMutable.value) return
  if (editorSnapshot() !== savedSnapshot.value) {
    const result = await confirmationDialog.value?.reveal({
      message: 'Restoring this version discards your unsaved edits.',
      confirmLabel: 'Restore',
    })
    if (!result || result.isCanceled) return
  }
  isSubmitting.value = true
  try {
    await contractWorkflowService.update({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
      name: item.name,
      description: item.description,
      start_date: item.start_date,
      exp_date: item.exp_date,
      exp_policy: item.exp_policy,
      exp_notice_period: item.exp_notice_period,
      contract_data: item.contract_data,
    })
    contract.value = await contractWorkflowService.retrieveById({ did: contract.value.did })
    applyContractDataToDraft(contract.value?.contract_data)
    savedSnapshot.value = editorSnapshot()
  } catch (error) {
    reportActionError(error, 'Restore version')
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
          savedSnapshot.value = editorSnapshot()
          templateEditorUiStore.setTemplateEditable(isEditableContractState(contract.value?.state))
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

// Only the fully retrieved template in the draft store is authoritative. The
// list entry selected above may omit template_data or contain a stale summary.
const loadedTemplatePartyRoles = computed(() =>
  dcsDraftStore.did === selectedTemplate.value?.did
    ? declaredPartyRoles({ 'dcs:policies': dcsDraftStore.policies })
    : [],
)
const templatePartyRoles = computed(() => {
  const roles = loadedTemplatePartyRoles.value
  if (roles.length !== 2) return []
  const options = roles.map((role) => CONTRACT_PARTY_ROLE_OPTIONS.find((option) => option.value === role))
  return options.every((option) => option !== undefined) ? options : []
})
const templateRoleState = computed<'loading' | 'ready' | 'empty' | 'error'>(() => {
  if (templateLoadState.value === 'loading') return 'loading'
  if (templateLoadState.value === 'error') return 'error'
  return templatePartyRoles.value.length === 2 ? 'ready' : 'empty'
})

function onTemplateLoadState(value: { did: string; state: 'loading' | 'loaded' | 'error' }) {
  if (value.did === selectedTemplate.value?.did) templateLoadState.value = value.state
}

const scrollStore = useScrollStore()

watch(selectedTemplate, (value) => {
  templateLoadState.value = 'loading'
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
      <ViewContractTemplateView v-else :did="selectedTemplate.did" :embedded="true" @load-state="onTemplateLoadState">
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
                <option :value="null">None</option>
                <option v-for="c in draftContracts" :key="c.did" :value="c.did">
                  {{ c.name ?? c.did }}
                </option>
              </select>
            </div>
          </div>
          <!-- DCS-FR-TR-19, DCS-FR-TR-20: the template user verifies the template before using it. -->
          <div>
            <button
              type="button"
              class="btn btn-outline btn-sm"
              :aria-expanded="showTemplateCheck"
              @click="showTemplateCheck = !showTemplateCheck"
            >
              Check template
            </button>
          </div>
          <TemplateVerificationPanel
            v-if="showTemplateCheck"
            :key="selectedTemplate.did"
            :did="selectedTemplate.did"
            :actions="['verify', 'provenance']"
          />
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
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor
                  :contract="contract"
                  :disabled="!isContractMutable"
                  :name-error="nameError ?? undefined"
                  :description-error="descriptionError ?? undefined"
                />
                <div v-if="isEditMode && contract" class="card mt-4 border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body">
                    <ContractEvidencePanel :contract="contract" />
                  </div>
                </div>
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
                        :disabled="!isContractMutable"
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

              <div v-if="isEditMode && contract" v-show="activeTab === 'diff'">
                <ContractHistoryDiffView
                  :contract-did="contract.did"
                  :contract-state="contract.state"
                  :current-contract-data="contract.contract_data"
                  :restorable="isContractMutable && !isSubmitting"
                  @restore="restoreVersion"
                />
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
          :party-roles="templatePartyRoles"
          :role-state="templateRoleState"
          :disabled="isSubmitting || !canSubmit"
          class="btn flex-1 btn-primary"
          @submit="createContract"
        />
        <button
          v-if="isEditMode && isContractMutable"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting || !canSubmit"
          @click="updateContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Update
        </button>
        <button
          v-if="canSubmitContract"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting"
          @click="submitContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Submit
        </button>
      </div>
    </div>
    <ConfirmationModal ref="confirmation-dialog" />
  </div>
</template>
