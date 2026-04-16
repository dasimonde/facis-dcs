<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import { computed } from 'vue'

const props = defineProps<{
  item: Contract
}>()

const authStore = useAuthStore()

const canEdit = computed(() => {
  return props.item.created_by === authStore.user?.username && props.item.state === ContractState.draft
})
</script>

<template>
  <li class="list-row min-w-0 w-full">
    <div class="list-col-grow card bg-base-200 card-border hover:bg-base-300 min-w-0 w-full">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ item.name }}</div>
          </div>
          <div class="badge badge-secondary">{{ item.state }}</div>
        </h2>
        <div class="flex justify-end">
          <div v-if="item.contract_version">Version: {{ item.contract_version }}</div>
        </div>
        <div class="flex justify-between min-w-0">
          <div>Creation date: {{ new Date(item.created_at).toLocaleDateString() }}</div>
          <div v-if="item.description" class="px-10 flex-1 min-w-0 truncate hidden sm:block">
            {{ item.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: item.did } }"
              class="btn btn-sm btn-primary rounded-box"
            >
              View
            </RouterLink>
            <RouterLink
              :to="
                canEdit
                  ? {
                      name: ROUTES.CONTRACTS.EDIT,
                      params: { did: item.did },
                    }
                  : '#'
              "
              class="btn btn-sm btn-primary rounded-box gap-2"
              :class="{ 'btn-disabled': !canEdit }"
            >
              Edit
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </li>
</template>
