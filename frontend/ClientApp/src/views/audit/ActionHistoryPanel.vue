<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ContractAuditList from '@/components/lists/contract/ContractAuditList.vue'
import TemplateAuditList from '@/components/lists/template/TemplateAuditList.vue'
import { contractTemplateService } from '@/services/contract-template-service'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import type { ContractAuditResponse } from '@/models/responses/contract-response'
import type { ContractTemplateAuditResponse } from '@/models/responses/template-response'

type DocumentType = 'contract' | 'template'

// A drill-down (for example from a compliance risk) opens the panel on its DID.
const props = defineProps<{ initialDid?: string }>()

const documentType = ref<DocumentType>('contract')
const documentDid = ref(props.initialDid ?? '')
const justification = ref('')
const loading = ref(false)
const loadError = ref('')
const contractAudits = ref<ContractAuditResponse>([])
const templateAudits = ref<ContractTemplateAuditResponse>([])
const loadedDid = ref('')
const loadedType = ref<DocumentType | null>(null)

const canView = computed(() => documentDid.value.trim().length > 0 && justification.value.trim().length > 0)
const hasLoaded = computed(() => loadedType.value !== null && loadedDid.value !== '')
const isEmpty = computed(() => {
  if (!hasLoaded.value) return false
  return loadedType.value === 'contract' ? contractAudits.value.length === 0 : templateAudits.value.length === 0
})

async function viewActivityHistory() {
  const did = documentDid.value.trim()
  if (!canView.value || loading.value) return
  if (loadedDid.value === did && loadedType.value === documentType.value) return

  loading.value = true
  loadError.value = ''
  contractAudits.value = []
  templateAudits.value = []

  try {
    if (documentType.value === 'contract') {
      contractAudits.value = await contractWorkflowService.audit({ did, justification: justification.value.trim() })
    } else {
      templateAudits.value = await contractTemplateService.audit({ did, justification: justification.value.trim() })
    }
    loadedDid.value = did
    loadedType.value = documentType.value
  } catch (err) {
    loadedDid.value = ''
    loadedType.value = null
    loadError.value = err instanceof Error ? err.message : 'The activity history could not be loaded'
  } finally {
    loading.value = false
  }
}

watch(documentType, () => {
  contractAudits.value = []
  templateAudits.value = []
  loadedDid.value = ''
  loadedType.value = null
  loadError.value = ''
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex min-w-0 flex-col gap-3 sm:flex-row sm:flex-wrap">
      <label class="form-control w-full sm:w-48">
        <span class="label-text mb-1">Scope</span>
        <select
          id="select-action-history-type"
          v-model="documentType"
          class="select-bordered select rounded-box"
          :disabled="loading"
        >
          <option value="contract">Contract</option>
          <option value="template">Template</option>
        </select>
      </label>

      <label class="form-control w-full sm:w-64">
        <span class="label-text mb-1">DID</span>
        <input
          id="input-action-history-did"
          v-model="documentDid"
          class="input-bordered input rounded-box"
          :disabled="loading"
          @keyup.enter="viewActivityHistory"
        />
      </label>

      <label class="form-control w-full sm:w-64">
        <span class="label-text mb-1">Justification</span>
        <input
          id="input-action-history-justification"
          v-model="justification"
          class="input-bordered input rounded-box"
          :disabled="loading"
          @keyup.enter="viewActivityHistory"
        />
      </label>

      <button
        class="btn rounded-box btn-primary sm:self-end"
        :disabled="loading || !canView"
        @click="viewActivityHistory"
      >
        <span v-if="loading" class="loading loading-sm loading-spinner"></span>
        <span v-else>View activity history</span>
      </button>
    </div>

    <div v-if="loading" class="p-4" role="status">Loading activity history...</div>
    <div v-else-if="loadError" class="alert rounded-box alert-error" role="alert">{{ loadError }}</div>
    <div v-else-if="isEmpty" class="alert rounded-box alert-info" role="status">
      No activity history for this document.
    </div>

    <div v-else-if="hasLoaded" class="min-w-0 overflow-x-auto rounded-box border border-base-content/10">
      <div class="border-b border-base-content/10 px-4 py-3">
        <h3 class="font-bold">Activity</h3>
        <div class="mt-1 font-mono text-xs break-all opacity-70">{{ loadedDid }}</div>
      </div>
      <div class="px-4 py-3">
        <ContractAuditList v-if="loadedType === 'contract'" :audits="contractAudits" />
        <TemplateAuditList v-else :audits="templateAudits" />
      </div>
    </div>
  </div>
</template>
