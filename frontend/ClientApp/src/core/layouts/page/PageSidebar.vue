<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, useTemplateRef } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { usePageStore } from '@core/store/page'
import facisLogo from '@/assets/FACIS_color.svg'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'

const router = useRouter()

const pageStore = usePageStore()
const { isSidebarCollapsed } = storeToRefs(pageStore)

const authStore = useAuthStore()
const { user } = storeToRefs(authStore)

const tooltipRef = useTemplateRef('tooltip-link')

const closeMobileDrawer = () => {
  const drawerToggle = document.getElementById(pageStore.pageSidebarId) as HTMLInputElement | null
  if (drawerToggle) drawerToggle.checked = false
}

const navigationRoutes = computed(() => {
  try {
    return router
      .getRoutes()
      .filter(
        (route) =>
          route.name &&
          !route.path.includes(':') &&
          route.meta?.name &&
          route.meta?.hideInSidebar !== true &&
          (!route.meta.roles || user.value?.roles?.some((role) => route.meta.roles?.includes(role))),
      )
      .sort((routeA, routeB) => (routeA.meta.order ?? 999) - (routeB.meta.order ?? 999))
  } catch {
    return []
  }
})

const showTooltip = (index: number) => {
  if (!isSidebarCollapsed.value) return

  tooltipRef.value?.[index]?.showPopover()
}

const hideTooltip = (index: number) => {
  tooltipRef.value?.[index]?.hidePopover()
}
</script>

<template>
  <div class="flex h-16 items-center justify-center overflow-hidden border-b border-base-content/10 px-4">
    <RouterLink
      :to="{ name: ROUTES.HOME }"
      aria-label="DCS Go to home page"
      class="text-2xl font-bold tracking-tight text-base-content uppercase"
    >
      <img :src="facisLogo" alt="FACIS Logo" class="h-10" />
    </RouterLink>
  </div>

  <nav class="overflow-x-hidden overflow-y-auto py-4" aria-label="Primary">
    <ul class="menu w-full gap-1 px-3 text-base-content">
      <li v-for="(route, index) in navigationRoutes" :key="route.path">
        <RouterLink
          :to="route.path"
          :class="['rounded-btn flex items-center gap-4 py-3', isSidebarCollapsed ? 'justify-center px-0' : 'px-4']"
          active-class="active bg-primary text-primary-content"
          :data-tip="isSidebarCollapsed ? route.meta?.name : ''"
          popovertarget="tooltip-link"
          :style="{ 'anchor-name': `--anchor-tooltip-${index}` }"
          :aria-describedby="isSidebarCollapsed ? `tooltip-link-${index}` : undefined"
          :aria-label="isSidebarCollapsed ? route.meta.name : undefined"
          @mouseenter="showTooltip(index)"
          @mouseleave="hideTooltip(index)"
          @focus="showTooltip(index)"
          @blur="hideTooltip(index)"
          @click="closeMobileDrawer"
        >
          <component :is="route.meta?.icon" class="h-6 w-6 shrink-0" aria-hidden="true" />
          <span v-if="!isSidebarCollapsed" class="font-medium whitespace-nowrap">
            {{ route.meta?.name }}
          </span>
        </RouterLink>
        <div
          :id="`tooltip-link-${index}`"
          ref="tooltip-link"
          popover
          role="tooltip"
          class="tooltip-link pointer-events-none overflow-visible rounded-md border-none bg-neutral px-3 py-1.5 text-xs text-neutral-content shadow-md"
          :style="{ 'position-anchor': `--anchor-tooltip-${index}` }"
        >
          <div class="tooltip-content">{{ route.meta.name }}</div>
        </div>
      </li>
    </ul>
  </nav>
</template>

<style scoped>
.tooltip-link::after {
  content: '';
  position: absolute;
  border-width: 4px;
  border-style: solid;
  right: 100%;
  top: 50%;
  transform: translateY(-50%);
  border-color: transparent;
  border-right-color: var(--color-neutral);
}

.tooltip-link:popover-open {
  display: block;
  opacity: 1;
  left: anchor(right);
  top: anchor(center);
  transform: translateY(-50%);
  margin-left: 0.5rem;
}

.tooltip-link:not(:popover-open) {
  display: none;
  opacity: 0;
}
</style>
