interface PageState {
  isSidebarCollapsed: boolean
  breadcrumbs?: BreadcrumbItem[]
  pageSidebarId: string
}

interface BreadcrumbItem {
  label: string
  path?: string
}

export type { PageState, BreadcrumbItem }
