<script setup lang="ts">
import { computed, ref } from 'vue'
import { ROUTES } from '@/router/router'
import type { Contract } from '@/models/contract/contract'

defineOptions({ name: 'ContractStructureTree' })

const props = withDefaults(
  defineProps<{
    rootDid: string
    contracts: Contract[]
    depth?: number
  }>(),
  { depth: 0 },
)

const expanded = ref(true)

const children = computed(() => props.contracts.filter((c) => c.parent_contract_did === props.rootDid))

const hasGrandchildren = (child: Contract) => props.contracts.some((c) => c.parent_contract_did === child.did)

const didSuffix = (did: string) => {
  const parts = did.split(':')
  return parts[parts.length - 1] ?? did
}
</script>

<template>
  <ul class="ml-4 space-y-1 border-l border-base-300 pl-4">
    <li v-for="child in children" :key="child.did">
      <div class="flex items-center gap-2">
        <button
          v-if="hasGrandchildren(child)"
          class="w-4 text-xs text-base-content/50 hover:text-base-content"
          @click="expanded = !expanded"
        >
          {{ expanded ? '▼' : '▶' }}
        </button>
        <span v-else class="w-4 text-center text-xs text-base-content/30">·</span>

        <RouterLink
          :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: child.did } }"
          class="link text-sm font-medium link-hover"
          target="_blank"
        >
          {{ child.name ?? didSuffix(child.did) }}
        </RouterLink>

        <span class="badge badge-ghost badge-xs">{{ child.state }}</span>
      </div>

      <ContractStructureTree
        v-if="expanded && hasGrandchildren(child)"
        :root-did="child.did"
        :contracts="contracts"
        :depth="depth + 1"
      />
    </li>
  </ul>
</template>
