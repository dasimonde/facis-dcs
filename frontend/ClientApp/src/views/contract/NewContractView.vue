<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import { useErrorStore } from '@/stores/error-store'
import { ContractState } from '@/types/contract-state'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import type { ContractData } from '@/models/contract-data'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import BuilderEditor from '@template-repository/components/BuilderEditor.vue'
import AddBlockModal from '@template-repository/components/builder-editor/AddBlockModal.vue'
import SemanticRulesEditor from '@template-repository/components/SemanticRulesEditor.vue'
import ClausesEditor from '@template-repository/components/ClausesEditor.vue'
import BuilderPreviewDialog from '@template-repository/components/builder-editor/BuilderPreviewDialog.vue'
import ViewContractTemplateView from '@/modules/template-repository/views/ViewContractTemplateView.vue'
import { useScrollStore } from '@/core/store/scroll'

const route = useRoute()
const router = useRouter()

const errorStore = useErrorStore()
const templatesStore = useContractTemplatesStore()
const { approvedOrPublishedTemplates, hasApprovedOrPublishedTemplates } = storeToRefs(templatesStore)
const templateDraftStore = useTemplateDraftStore()
const contractContentValuesStore = useContractContentValuesStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)

const did = ref<string | null>(null)
const isEditMode = computed(() => !!route.params.did || !!did.value)
const isSubmitting = ref(false)
const selectedTemplate: Ref<PartialContractTemplate | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)

const contract: Ref<Contract | null> = ref(null)

const canSubmit = computed(
  () => isEditMode.value || (hasApprovedOrPublishedTemplates.value && selectedTemplate.value !== null),
)

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  if (!isEditMode.value) return null
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const tabs = computed(() => contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft))

const submit = async () => {
  isSubmitting.value = true
  try {
    if (!isEditMode.value && !!selectedTemplate.value) {
      const response = await contractWorkflowService.create({ did: selectedTemplate.value.did })
      did.value = response.did
      errorStore.add('Contract created.', 'info')
    } else if (contract.value) {
      const contractData: ContractData = {
        documentOutline: templateDraftStore.documentOutline,
        documentBlocks: templateDraftStore.documentBlocks,
        semanticConditions: templateDraftStore.semanticConditions,
        subTemplateSnapshots: templateDraftStore.subTemplateSnapshots,
        templateDataVersion: templateDraftStore.templateDataVersion,
        semanticConditionValues: contractContentValuesStore.semanticConditionValues,
      }
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        exp_notice_period: contract.value.exp_notice_period,
        exp_policy: contract.value.exp_policy,
        name: contract.value.name,
        description: contract.value.description,
        contract_data: contractData,
      })
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    console.error('Submission failed', error)
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
          const uneditableStates = [ContractState.approved, ContractState.terminated].map((s) => s.toLowerCase())
          templateEditorUiStore.setTemplateEditable(
            !uneditableStates.includes(contract.value?.state.toLowerCase() ?? ''),
          )
        }
      } catch (err: unknown) {
        console.error('Failed to load contract', err)
      }
    } else if (!hasApprovedOrPublishedTemplates.value) {
      await templatesStore.loadTemplates()
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

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract' })
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
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div v-if="!isEditMode" class="px-6 py-12">
      <div class="flex justify-center">
        <select v-model="selectedTemplate" class="select" :disabled="!hasApprovedOrPublishedTemplates">
          <option :value="null" disabled selected>
            {{ hasApprovedOrPublishedTemplates ? 'Pick a template' : 'No templates available' }}
          </option>
          <option v-for="template in approvedOrPublishedTemplates" :key="template.did" :value="template">
            {{ template.name }}
          </option>
        </select>
      </div>
      <div v-if="selectedTemplate" class="pt-10">
        <ViewContractTemplateView :did="selectedTemplate.did" />
      </div>
    </div>
    <div v-else-if="!!contract">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">
              {{ isEditMode ? 'Update Contract' : 'Create Contract' }}
            </p>
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
                <ContractDetailsEditor :contract="contract" />
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
              <!-- SEMANTIC RULES TAB -->
              <div v-show="activeTab === 'semantic'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <SemanticRulesEditor />
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
        <button class="btn btn-outline md:w-32" @click="$router.back()">Cancel</button>
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting || !canSubmit" @click="submit">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          {{ isEditMode ? 'Update' : 'Create' }}
        </button>
      </div>
    </div>
  </div>
</template>
