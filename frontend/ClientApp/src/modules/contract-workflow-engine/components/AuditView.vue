<script setup lang="ts">
import ContractAuditList from '@/components/lists/contract/ContractAuditList.vue'
import type { ContractAuditResponse } from '@/models/responses/contract-response'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ref, watch, type Ref } from 'vue'
import { useContractEditorUiStore } from '../store/contractEditorUiStore'
import { useRoute } from 'vue-router'

const route = useRoute()
const editorStore = useContractEditorUiStore()
const data: Ref<ContractAuditResponse> = ref([])

const isLoading = ref(false)

const loadAudit = async () => {
  const did = route.params.did
  if (!did || Array.isArray(did)) return
  try {
    isLoading.value = true
    data.value = await contractWorkflowService.audit({ did })
  } catch (err) {
    console.error('Audit failed', err)
  } finally {
    isLoading.value = false
  }
}

watch(
  () => editorStore.activeTab === 'audit',
  async (value) => {
    if (value) await loadAudit()
    else data.value = []
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="isLoading" class="loading loading-sm loading-spinner"></div>
  <div v-else-if="data.length < 1">No audit data</div>
  <ContractAuditList v-else :audits="data" />
</template>
