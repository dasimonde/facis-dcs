<script setup lang="ts">
import { computed, type Ref, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { contractAuditEventDisplayText, contractAuditEventTransition } from '@/utils/contract-audit-event-display'
import type { ContractLifecycleEvent } from '@/models/responses/contract-response'

const route = useRoute()
const editorStore = useContractEditorUiStore()
const data: Ref<ContractLifecycleEvent[]> = ref([])
const isLoading = ref(false)
const loadError = ref('')

watch(
  [() => editorStore.activeTab, () => route.params.did],
  async ([tab, did], _, onCleanup) => {
    let current = true
    onCleanup(() => {
      current = false
    })
    data.value = []
    loadError.value = ''
    isLoading.value = false
    if (tab !== 'history' || typeof did !== 'string') return
    isLoading.value = true
    try {
      const result = await contractWorkflowService.lifecycle(did)
      if (current) data.value = result
    } catch (err) {
      if (current) loadError.value = err instanceof Error ? err.message : 'The lifecycle history could not be loaded'
    } finally {
      if (current) isLoading.value = false
    }
  },
  { immediate: true },
)

const rows = computed(() =>
  data.value.map((event, index) => {
    const date = new Date(event.occurred_at)
    return {
      key: `${index}-${event.event_type}-${event.occurred_at}`,
      title: contractAuditEventDisplayText(event.event_type, event),
      occurredAt: Number.isNaN(date.getTime()) ? event.occurred_at : date.toLocaleString(),
      actor: event.actor,
      transition: contractAuditEventTransition(event),
    }
  }),
)
</script>

<template>
  <div v-if="isLoading" class="loading loading-sm loading-spinner"></div>
  <div v-else-if="loadError" role="alert" class="alert alert-error">{{ loadError }}</div>
  <div v-else-if="rows.length < 1">No lifecycle history</div>
  <ul v-else class="divide-y divide-base-300" data-testid="lifecycle-history">
    <li v-for="row in rows" :key="row.key" class="py-3 first:pt-0 last:pb-0">
      <div class="text-sm font-medium text-base-content">{{ row.title }}</div>
      <div class="mt-0.5 text-xs text-base-content/70">{{ row.occurredAt }}</div>
      <div v-if="row.actor" class="mt-1 font-mono text-xs break-all text-base-content/80">By: {{ row.actor }}</div>
      <div v-if="row.transition" class="mt-1 text-sm text-base-content/90">Transition: {{ row.transition }}</div>
    </li>
  </ul>
</template>
