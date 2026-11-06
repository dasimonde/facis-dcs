<script setup lang="ts">
import { ref } from 'vue'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import ContractAttention from '@/components/lists/contract/ContractAttention.vue'
import ContractList from '@/components/lists/contract/ContractList.vue'
import { ROUTES } from '@/router/router'

const { isCreator } = useContractPermissions()
// A contract change reloads the attention list (DCS-FR-CSA-23).
const attentionVersion = ref(0)
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="mb-4 flex shrink-0 justify-between border-b border-base-content/10 bg-base-100 p-4">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name }}
      </h2>

      <RouterLink
        v-if="isCreator"
        v-slot="{ route }"
        :to="{ name: ROUTES.CONTRACTS.NEW }"
        class="btn gap-2 self-end btn-primary"
      >
        {{ route.meta.name }}
      </RouterLink>
      <div v-else></div>
    </div>
    <ContractAttention :version="attentionVersion" />
    <div class="min-h-0 flex-1 overflow-hidden">
      <ContractList @change="attentionVersion++" />
    </div>
  </div>
</template>
