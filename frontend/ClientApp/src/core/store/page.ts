import { defineStore } from 'pinia'
import type { PageState } from '@core/models/page-store'

const storeId = 'page'
const defaultState: PageState = {
  pageSidebarId: 'main-drawer',
  isSidebarCollapsed: false,
  breadcrumbs: [],
}

export const usePageStore = defineStore(storeId, {
  state: (): PageState => defaultState,
  getters: {},
  actions: {
    toggleSidebar() {
      this.isSidebarCollapsed = !this.isSidebarCollapsed
    },
  },
})
