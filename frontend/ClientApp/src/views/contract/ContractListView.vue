<script setup lang="ts">
import ContractList from '@/components/lists/contract/ContractList.vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'
import { ROUTES } from '@/router/router'

const { isCreator } = useContractPermissions()
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
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
  <ContractList />
</template>
