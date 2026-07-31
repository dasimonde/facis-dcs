<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { usePageStore } from '@core/store/page'
import AuthButton from '@/components/auth/AuthButton.vue'
import PermissionList from '@/components/PermissionList.vue'

const pageStore = usePageStore()
const { isSidebarCollapsed, pageSidebarId } = storeToRefs(pageStore)
</script>

<template>
  <div class="flex-none">
    <label :for="pageSidebarId" class="btn btn-square btn-ghost lg:hidden" aria-label="Open navigation menu">
      <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="h-6 w-6 stroke-current">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
      </svg>
    </label>

    <button
      type="button"
      class="btn hidden btn-square btn-ghost lg:inline-flex"
      :aria-label="isSidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
      @click="pageStore.toggleSidebar()"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        fill="none"
        viewBox="0 0 24 24"
        stroke-width="1.5"
        stroke="currentColor"
        :class="['h-6 w-6 transition-transform duration-300', isSidebarCollapsed ? 'rotate-180' : '']"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
      </svg>
    </button>
  </div>

  <div class="flex-1 px-4 text-lg font-bold">
    DCS
    <span class="ml-2 font-normal text-base-content/70">| Dashboard</span>
  </div>

  <PermissionList class="mr-2" />
  <AuthButton class="btn flex-none gap-2 btn-outline btn-sm" />
</template>
