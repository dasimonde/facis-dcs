<script setup lang="ts">
import { useAuthStore } from '@/stores/auth-store'
import { toProperCase } from '@/utils/string'
import { computed } from 'vue'

const authStore = useAuthStore()
const roles = computed(() => [...(authStore.user?.roles ?? [])].sort())

const formatRole = (role: string): string => toProperCase(role)
</script>

<template>
  <div class="relative">
    <button id="perm-btn-user" class="btn gap-2 btn-outline btn-sm" popovertarget="perm-popover-user">
      User Details
    </button>

    <div
      id="perm-popover-user"
      class="menu dropdown dropdown-end mt-2 ml-2 w-auto max-w-[95vw] min-w-[16rem] rounded-box border border-base-300 bg-base-100 p-4 shadow-lg sm:ml-0 sm:min-w-[20rem] md:max-w-lg md:min-w-[24rem] lg:min-w-md"
      popover
      anchor="perm-btn-user"
    >
      <div class="mb-3 border-b border-base-300 pb-3">
        <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Issuer</p>
        <p class="text-xs break-all text-base-content/60">{{ authStore.user?.issuer }}</p>
      </div>

      <div class="mb-3 border-b border-base-300 pb-3">
        <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Holder</p>
        <p class="text-xs break-all text-base-content/60">{{ authStore.user?.holder }}</p>
      </div>

      <p class="mb-2 text-xs font-bold text-base-content/50 uppercase">Permissions</p>
      <ul class="flex flex-col gap-1">
        <li v-for="role in roles" :key="role" class="w-full justify-start">
          {{ formatRole(role) }}
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
#perm-btn-user {
  anchor-name: --anchor-perm-user;
}

#perm-popover-user {
  position-anchor: --anchor-perm-user;
}
</style>
