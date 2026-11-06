<script setup lang="ts">
import { type Ref, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ContractAuditList from '@/components/lists/contract/ContractAuditList.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import ContractEvidencePanel from './ContractEvidencePanel.vue'
import type { Contract } from '@/models/contract/contract'
import type { DcsBlock, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { ContractAuditResponse } from '@/models/responses/contract-response'

const props = defineProps<{ contract: Contract; blocks?: DcsBlock[]; layout?: DcsLayoutNode[] }>()
const route = useRoute()
const editorStore = useContractEditorUiStore()
const data: Ref<ContractAuditResponse> = ref([])

const isLoading = ref(false)
const loadError = ref('')
const justification = ref('')

// A response that arrives after the tab or the contract changed is dropped.
let requestSequence = 0

const loadAudit = async () => {
  const did = route.params.did
  if (typeof did !== 'string') return
  const sequence = ++requestSequence
  data.value = []
  loadError.value = ''
  isLoading.value = true
  try {
    const result = await contractWorkflowService.audit({ did, justification: justification.value.trim() })
    if (sequence === requestSequence) data.value = result
  } catch (err) {
    if (sequence === requestSequence)
      loadError.value = err instanceof Error ? err.message : 'The audit trail could not be loaded'
  } finally {
    if (sequence === requestSequence) isLoading.value = false
  }
}

watch([() => editorStore.activeTab, () => route.params.did], () => {
  requestSequence++
  data.value = []
  loadError.value = ''
  isLoading.value = false
})
</script>

<template>
  <ContractEvidencePanel
    v-if="editorStore.activeTab === 'audit' && route.params.did === props.contract.did"
    :key="props.contract.did"
    :contract="props.contract"
  />
  <form class="mb-3 flex items-end gap-2" @submit.prevent="loadAudit">
    <label class="form-control grow">
      <span class="label-text mb-1">Audit justification</span>
      <input v-model="justification" class="input-bordered input input-sm" required />
    </label>
    <button type="submit" class="btn btn-sm btn-primary" :disabled="isLoading || !justification.trim()">
      Load audit trail
    </button>
  </form>
  <div v-if="isLoading" class="loading loading-sm loading-spinner"></div>
  <div v-else-if="loadError" role="alert" class="alert alert-error">{{ loadError }}</div>
  <div v-else-if="data.length < 1">No audit data</div>
  <ContractAuditList v-else :audits="data" :blocks="blocks" :layout="layout" />
</template>
