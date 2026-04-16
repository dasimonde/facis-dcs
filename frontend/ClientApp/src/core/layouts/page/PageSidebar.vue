<template>
  <div class="flex items-center h-16 px-4 overflow-hidden">
    <RouterLink :to="{ name: ROUTES.HOME }" #default="{ route }" class="font-bold text-2xl tracking-tight text-base-content uppercase">
      {{ route.meta.name }}
    </RouterLink>
  </div>

  <nav class="overflow-y-auto overflow-x-hidden py-4">
    <ul class="menu px-3 gap-1 w-full text-base-content">
      <li v-for="route in navigationRoutes" :key="route.path">
        <RouterLink :to="route.path" @click="closeMobileDrawer" :class="[
          'flex items-center gap-4 py-3 rounded-btn',
          isSidebarCollapsed ? 'justify-center px-0' : 'px-4'
        ]" active-class="active bg-primary text-primary-content" :data-tip="isSidebarCollapsed ? route.meta?.name : ''">
          <component :is="route.meta?.icon" class="w-6 h-6 shrink-0" aria-hidden="true" />
          <span v-if="!isSidebarCollapsed" class="font-medium whitespace-nowrap">
            {{ route.meta?.name }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </nav>

  <div class="flex-1"></div>

  <div class="p-4 border-t border-base-content/10 bg-base-300/20">
    <div :class="['flex items-center gap-3', isSidebarCollapsed ? 'justify-center' : 'px-2']">
      <div class="avatar">
        <div class="w-10 rounded-full ring ring-primary ring-offset-base-100 ring-offset-2">
          <img
            src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?auto=format&fit=facearea&facepad=2&w=128&h=128&q=80"
            alt="Profile" />
        </div>
      </div>
      <div v-if="!isSidebarCollapsed && user" class="overflow-hidden">
        <p class="text-sm font-bold truncate">{{ user.name }}</p>
        <p class="text-xs opacity-60">{{ user.username }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { usePageStore } from '@core/store/page'
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

const router = useRouter()

const pageStore = usePageStore()
const { isSidebarCollapsed } = storeToRefs(pageStore)

const authStore = useAuthStore()
const { user } = storeToRefs(authStore)

const closeMobileDrawer = () => {
  const drawerToggle = document.getElementById(pageStore.pageSidebarId) as HTMLInputElement | null
  if (drawerToggle) drawerToggle.checked = false
}

const navigationRoutes = computed(() => {
  try {
    return router.getRoutes()
      .filter(route =>
        route.name &&
        !route.path.includes(':') &&
        route.meta?.name &&
        route.meta?.hideInSidebar !== true &&
        (!route.meta.roles || user.value?.roles?.some(role => route.meta.roles?.includes(role)))
      )
      .sort((routeA, routeB) => (routeA.meta.order || 999) - (routeB.meta.order || 999))
  } catch (e) {
    return []
  }
})
</script>
