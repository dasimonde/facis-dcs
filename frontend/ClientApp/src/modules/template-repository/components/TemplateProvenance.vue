<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { contractTemplateService } from '@/services/contract-template-service'
import type { ContractTemplateHistoryResponse } from '@/models/responses/template-response'

const store = useDcsDraftStore()
const { did, created_by, created_at, updated_at } = storeToRefs(store)
const { hasTemplateRole } = useTemplatePermissions()

function formatWhen(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

const rows = computed(() => [
  { label: 'Template ID', value: did.value ?? '—' },
  { label: 'creator', value: created_by.value !== '' ? created_by.value : '—' },
  { label: 'creation timestamp', value: formatWhen(created_at.value) },
  { label: 'last change timestamp', value: formatWhen(updated_at.value) },
])

const history = ref<ContractTemplateHistoryResponse>([])
const historyError = ref('')

// The history endpoint returns one snapshot per state change, in no defined
// order, so the entries are sorted oldest first.
watch(
  did,
  async (value) => {
    history.value = []
    historyError.value = ''
    if (!value || !hasTemplateRole.value) return
    try {
      const entries = await contractTemplateService.retrieveHistory(value)
      if (value !== did.value) return
      history.value = [...entries].sort((a, b) => Date.parse(a.updated_at) - Date.parse(b.updated_at))
    } catch {
      if (value === did.value) historyError.value = 'The template history could not be loaded.'
    }
  },
  { immediate: true },
)
</script>

<template>
  <dl
    data-testid="template-provenance"
    class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 rounded-box border border-base-content/10 bg-base-200/40 p-4 text-sm"
  >
    <template v-for="row in rows" :key="row.label">
      <dt class="font-medium text-base-content/70">{{ row.label }}</dt>
      <dd class="min-w-0 break-all">{{ row.value }}</dd>
    </template>
  </dl>

  <div v-if="hasTemplateRole" class="overflow-x-auto">
    <h3 class="mb-2 text-sm font-medium">Version history</h3>
    <div v-if="historyError" role="alert" class="alert text-sm alert-error">{{ historyError }}</div>
    <p v-else-if="history.length === 0" class="text-sm text-base-content/60">No history entries recorded.</p>
    <table v-else class="table table-sm" data-testid="template-history">
      <thead>
        <tr>
          <th>Version</th>
          <th>State</th>
          <th>Changed at</th>
          <th>Created by</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(entry, i) in history" :key="i" data-testid="template-history-entry">
          <td>{{ entry.version }}</td>
          <td>{{ entry.state }}</td>
          <td>{{ formatWhen(entry.updated_at) }}</td>
          <td class="break-all">{{ entry.created_by }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
