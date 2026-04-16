import { getUIBasePath } from '@/config'
import ApproveContractTemplateView from '@/modules/template-repository/views/ApproveContractTemplateView.vue'
import ReviewContractTemplateView from '@/modules/template-repository/views/ReviewContractTemplateView.vue'
import ViewContractTemplateView from '@/modules/template-repository/views/ViewContractTemplateView.vue'
import { authenticationService } from '@/services/authentication-service'
import { useAuthStore } from '@/stores/auth-store'
import AuthSuccessView from '@/views/auth/AuthSuccessView.vue'
import LoginView from '@/views/auth/LoginView.vue'
import ContractTemplateListView from '@/views/contract-template-list/ContractTemplateListView.vue'
import ContractListView from '@/views/contract/ContractListView.vue'
import NewContractView from '@/views/contract/NewContractView.vue'
import ViewContractView from '@/views/contract/ViewContractView.vue'
import TaskListView from '@/views/task/TaskListView.vue'
import TemplateCatalogueAdminView from '@/views/template-repository/TemplateCatalogueAdminView.vue'
import {
  ChatBubbleLeftRightIcon,
  DocumentCheckIcon,
  DocumentDuplicateIcon,
  DocumentMagnifyingGlassIcon,
  DocumentTextIcon,
} from '@heroicons/vue/20/solid'
import NewContractTemplateView from '@template-repository/views/NewContractTemplateView.vue'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const ROUTES = {
  HOME: 'home',
  TEMPLATES: {
    LIST: 'templates.list',
    NEW: 'templates.new',
    EDIT: 'templates.edit',
    VIEW: 'templates.view',
    REVIEW: 'templates.review',
    APPROVE: 'templates.approve',
  },
  TASKS: {
    REVIEWS: 'tasks.reviews',
    APPROVALS: 'tasks.approvals',
    NEGOTIATIONS: 'tasks.negotiations',
  },
  TEMPLATE_CATALOGUES: {
    ADMIN: 'template.catalogues.admin',
  },
  AUTH: {
    SUCCESS: 'auth.success',
  },
  CONTRACTS: {
    LIST: 'contracts.list',
    NEW: 'contracts.new',
    EDIT: 'contracts.edit',
    VIEW: 'contracts.view',
  },
} as const

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: ROUTES.HOME,
    meta: { name: 'DCS', hideInSidebar: true, requiresAuth: false, layout: 'blank', title: 'DCS' },
    component: LoginView,
  },
  {
    path: '/templates',
    name: ROUTES.TEMPLATES.LIST,
    component: ContractTemplateListView,
    meta: {
      name: 'Templates',
      icon: DocumentTextIcon,
      requiresAuth: true,
      title: 'DCS - Templates',
      order: 1,
    },
  },
  {
    path: '/templates/new',
    name: ROUTES.TEMPLATES.NEW,
    component: NewContractTemplateView,
    meta: {
      name: 'New Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - New Template',
      roles: ['TEMPLATE_CREATOR'],
    },
  },
  {
    path: '/templates/edit/:did',
    name: ROUTES.TEMPLATES.EDIT,
    component: NewContractTemplateView,
    meta: {
      name: 'Edit Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Edit Template',
      roles: ['TEMPLATE_CREATOR', 'TEMPLATE_REVIEWER'],
    },
  },
  {
    path: '/templates/view/:did',
    name: ROUTES.TEMPLATES.VIEW,
    component: ViewContractTemplateView,
    meta: { name: 'View Template', hideInSidebar: true, requiresAuth: true, title: 'DCS - View Template' },
  },
  {
    path: '/templates/review/:did',
    name: ROUTES.TEMPLATES.REVIEW,
    component: ReviewContractTemplateView,
    meta: {
      name: 'Review Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Review Template',
      roles: ['TEMPLATE_REVIEWER'],
    },
  },
  {
    path: '/templates/approve/:did',
    name: ROUTES.TEMPLATES.APPROVE,
    component: ApproveContractTemplateView,
    meta: {
      name: 'Approve Template',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Approve Template',
      roles: ['TEMPLATE_APPROVER'],
    },
  },
  {
    path: '/tasks/reviews',
    name: ROUTES.TASKS.REVIEWS,
    component: TaskListView,
    meta: {
      name: 'Review Tasks',
      icon: DocumentMagnifyingGlassIcon,
      requiresAuth: true,
      title: 'DCS - Review Tasks',
      order: 3.1,
      roles: ['TEMPLATE_REVIEWER', 'CONTRACT_REVIEWER'],
    },
  },
  {
    path: '/tasks/approvals',
    name: ROUTES.TASKS.APPROVALS,
    component: TaskListView,
    meta: {
      name: 'Approval Tasks',
      icon: DocumentCheckIcon,
      requiresAuth: true,
      title: 'DCS - Approval Tasks',
      order: 3.2,
      roles: ['TEMPLATE_APPROVER', 'CONTRACT_APPROVER'],
    },
  },
  {
    path: '/tasks/negotiations',
    name: ROUTES.TASKS.NEGOTIATIONS,
    component: TaskListView,
    meta: {
      name: 'Negotiation Tasks',
      icon: ChatBubbleLeftRightIcon,
      requiresAuth: true,
      title: 'DCS - Negotiation Tasks',
      order: 3.3,
      roles: ['CONTRACT_NEGOTIATOR'],
    },
  },
  {
    path: '/catalogues/admin',
    name: ROUTES.TEMPLATE_CATALOGUES.ADMIN,
    component: TemplateCatalogueAdminView,
    meta: {
      name: 'Template Catalogue Admin',
      icon: DocumentTextIcon,
      requiresAuth: true,
      title: 'DCS - Template Catalogue Admin',
      order: 4,
      roles: ['SYSTEM_ADMINISTRATOR'],
    },
  },
  {
    path: '/contracts',
    name: ROUTES.CONTRACTS.LIST,
    component: ContractListView,
    meta: {
      name: 'Contracts',
      icon: DocumentDuplicateIcon,
      requiresAuth: true,
      title: 'DCS - Contracts',
      order: 2,
      roles: ['CONTRACT_CREATOR', 'CONTRACT_REVIEWER', 'CONTRACT_APPROVER', 'CONTRACT_MANAGER'],
    },
  },
  {
    path: '/contracts/new',
    name: ROUTES.CONTRACTS.NEW,
    component: NewContractView,
    meta: {
      name: 'New Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - New Contract',
      roles: ['CONTRACT_CREATOR'],
    },
  },
  {
    path: '/contracts/edit/:did',
    name: ROUTES.CONTRACTS.EDIT,
    component: NewContractView,
    meta: {
      name: 'Edit Contract',
      hideInSidebar: true,
      requiresAuth: true,
      title: 'DCS - Edit Contract',
      roles: ['CONTRACT_CREATOR'],
    },
  },
  {
    path: '/contracts/view/:did',
    name: ROUTES.CONTRACTS.VIEW,
    component: ViewContractView,
    meta: { name: 'View Contract', hideInSidebar: true, requiresAuth: true, title: 'DCS - View Contract' },
  },
  {
    path: '/auth/success',
    name: ROUTES.AUTH.SUCCESS,
    meta: { hideInSidebar: true, requiresAuth: false, layout: 'blank', title: 'DCS - Auth Success' },
    component: AuthSuccessView,
  },
]

const router = createRouter({
  history: createWebHistory(getUIBasePath()),
  routes: routes,
})

router.beforeEach(async (to) => {
  if (to.meta.requiresAuth === false) {
    return true
  }

  const authStore = useAuthStore()
  if (authStore.isAuthenticated) {
    return true
  }

  await authenticationService.refresh()
  if (authStore.isAuthenticated) {
    return true
  }

  const loginUrl = await authenticationService.loginPath()
  if (loginUrl) {
    window.location.href = loginUrl
    return false
  }

  return { name: ROUTES.HOME }
})

router.beforeEach((to) => {
  if (!to.meta.roles) {
    return true
  }
  const authStore = useAuthStore()
  const hasAuthorizedRole = authStore.user?.roles?.some((role) => to.meta.roles?.includes(role)) ?? false
  if (!hasAuthorizedRole) {
    return { name: ROUTES.HOME }
  }
})

export { router, ROUTES }
