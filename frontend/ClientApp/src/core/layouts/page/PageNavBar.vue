<template>
  <div class="flex-none">
    <label :for="pageSidebarId" class="btn btn-square btn-ghost lg:hidden">
      <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="h-6 w-6 stroke-current">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
      </svg>
    </label>

    <button class="btn hidden btn-square btn-ghost lg:inline-flex" @click="pageStore.toggleSidebar()">
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
    <span class="ml-2 font-normal opacity-50">| Dashboard</span>
  </div>

  <AuthButton class="flex-none" />
  <div class="flex items-center gap-3 px-2">
    <div class="avatar">
      <div class="w-10 rounded-full ring ring-primary ring-offset-2 ring-offset-base-100">
        <img
          src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?auto=format&fit=facearea&facepad=2&w=128&h=128&q=80"
          alt="Profile"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import AuthButton from '@/components/AuthButton.vue'
import { usePageStore } from '@core/store/page'
import { storeToRefs } from 'pinia'

const pageStore = usePageStore()
const { isSidebarCollapsed, pageSidebarId } = storeToRefs(pageStore)
</script>
