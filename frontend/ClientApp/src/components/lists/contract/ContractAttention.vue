<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import http from '@/api/http'
import { ROUTES } from '@/router/router'

/** A contract that needs its owner's attention (GET /contract/attention). */
export interface ContractAttentionItem {
  did: string
  name?: string
  state: string
  reason: 'renewal_due' | 'expired' | 'terminated'
  exp_date?: string
  terminated_at?: string
}

/**
 * Contracts that need attention (DCS-FR-CSA-23): the notice period before
 * expiry has started, or the contract expired or was terminated in the last
 * 30 days. The server computes the list from the contract data and the read
 * policy on each load. A change of version reloads it.
 */
const props = defineProps<{ version?: number }>()
const items = ref<ContractAttentionItem[]>([])

const reasonLabel: Record<ContractAttentionItem['reason'], string> = {
  renewal_due: 'Renewal due',
  expired: 'Expired',
  terminated: 'Terminated',
}

async function load() {
  try {
    items.value = (await http.get<ContractAttentionItem[]>('/contract/attention')).data ?? []
  } catch {
    // the shared http interceptor already surfaced the server's error
  }
}

onMounted(load)
watch(() => props.version, load)

function when(item: ContractAttentionItem): string {
  const value = item.reason === 'terminated' ? item.terminated_at : item.exp_date
  if (!value) return ''
  const prefix = item.reason === 'renewal_due' ? 'expires' : item.reason
  return `${prefix} ${new Date(value).toLocaleDateString()}`
}
</script>

<template>
  <section
    v-if="items.length > 0"
    class="mx-4 mb-4 rounded-box border border-warning bg-base-100 p-4"
    aria-labelledby="contract-attention-title"
    data-testid="contract-attention"
  >
    <h3 id="contract-attention-title" class="font-semibold">Needs attention ({{ items.length }})</h3>
    <ul class="mt-2 space-y-1 text-sm">
      <li v-for="item in items" :key="item.did" class="flex flex-wrap items-center gap-2">
        <span
          class="badge badge-sm"
          :class="item.reason === 'renewal_due' ? 'badge-warning' : 'badge-ghost'"
          :data-testid="`contract-attention-${item.did}`"
        >
          {{ reasonLabel[item.reason] }}
        </span>
        <RouterLink :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: item.did } }" class="link wrap-anywhere">
          {{ item.name || item.did }}
        </RouterLink>
        <span class="opacity-70">{{ when(item) }}</span>
      </li>
    </ul>
  </section>
</template>
