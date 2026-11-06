<script setup lang="ts">
import { type Ref, ref, watch } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import TemplateAuditList from '@/components/lists/template/TemplateAuditList.vue'
import { contractTemplateService } from '@/services/contract-template-service'
import type { ContractTemplateAuditResponse } from '@/models/responses/template-response'

const store = useDcsDraftStore()
const editorStore = useTemplateEditorUiStore()
const data: Ref<ContractTemplateAuditResponse> = ref([])

const isLoading = ref(false)
const loadError = ref('')
const justification = ref('')

const loadAudit = async () => {
  const did = store.did
  const updated_at = store.updated_at
  if (!did || !updated_at) return

  loadError.value = ''
  try {
    isLoading.value = true
    data.value = await contractTemplateService.audit({ did, updated_at, justification: justification.value.trim() })
  } catch (err) {
    data.value = []
    loadError.value = err instanceof Error ? err.message : 'The audit trail could not be loaded'
  } finally {
    isLoading.value = false
  }
}

watch(
  () => editorStore.activeTab === 'audit',
  (value) => {
    if (!value) {
      data.value = []
      loadError.value = ''
    }
  },
)
</script>

<template>
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
  <TemplateAuditList v-else :audits="data" />
</template>
