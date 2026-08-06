<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted, useTemplateRef } from 'vue'
import { RouterView } from 'vue-router'
import PageNavBar from '@core/layouts/page/PageNavBar.vue'
import PageSidebar from '@core/layouts/page/PageSidebar.vue'
import { usePageStore } from '@core/store/page'
import { useScrollStore } from '@core/store/scroll'

const scrollContainer = useTemplateRef<HTMLElement>('scroll-container')

const pageStore = usePageStore()
const { isSidebarCollapsed, pageSidebarId } = storeToRefs(pageStore)
const scrollStore = useScrollStore()

// Functional classes for DaisyUI drawer behavior (structure/toggle), not layout or styling
const drawerClasses = {
  root: ['drawer', 'lg:drawer-open'],
  header: ['drawer-content'],
  sidebar: ['drawer-side'],
}

onMounted(() => {
  scrollStore.scrollContainer = scrollContainer.value
})
</script>

<template>
  <div :class="[drawerClasses.root, 'min-h-screen']">
    <input :id="pageSidebarId" type="checkbox" class="drawer-toggle" />
    <div :class="[drawerClasses.header, 'flex h-screen min-w-0 flex-col overflow-hidden bg-base-100']">
      <!-- Navbar -->
      <header class="navbar sticky top-0 z-30 w-full border-b border-base-content/10 bg-base-100">
        <slot name="navbar">
          <PageNavBar />
        </slot>
      </header>

      <!-- Main Content -->
      <main
        ref="scroll-container"
        class="min-w-0 grow overflow-x-hidden overflow-y-auto bg-base-200 outline-none"
        tabindex="-1"
      >
        <slot>
          <RouterView />
        </slot>
      </main>
    </div>

    <!-- Sidebar -->
    <div :class="[drawerClasses.sidebar, 'z-40']">
      <label :for="pageSidebarId" aria-label="close sidebar" class="drawer-overlay"></label>
      <aside
        :class="[
          'flex min-h-full flex-col border-r border-base-content/5 bg-base-200 transition-all duration-300 ease-in-out',
          isSidebarCollapsed ? 'lg:w-20' : 'w-72',
        ]"
      >
        <slot name="sidebar">
          <PageSidebar />
        </slot>
      </aside>
    </div>
  </div>
</template>
