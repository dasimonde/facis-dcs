import type { UserRole } from '@/types/user-role'
import type { FunctionalComponent, HTMLAttributes, VNodeProps } from 'vue'
import 'vue-router'

export { }

declare module 'vue-router' {
  interface RouteMeta {
    name?: string
    hideInSidebar?: boolean
    icon?: FunctionalComponent<HTMLAttributes & VNodeProps>
    requiresAuth: boolean
    /** This is used for setting the page title in useSyncPageTitle composable */
    title: string
    layout?: 'blank'
    order?: number
    roles?: UserRole[]
  }
}
