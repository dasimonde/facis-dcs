import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { router, ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import WorkflowStageBanner from './WorkflowStageBanner.vue'
import type { UserRole } from '@/types/user-role'

/**
 * The banner narrates the stage and names the role that acts next, which is
 * often not the reader's. A CTA whose destination the route guard would refuse
 * used to render as the page's primary button and eject the user to the front
 * page with no message.
 */

function mountBanner(roles: UserRole[], routeName: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  return mount(WorkflowStageBanner, {
    props: {
      steps: [{ key: 'APPROVED', label: 'Approved' }],
      currentKey: 'APPROVED',
      headline: 'Approved',
      narrative: 'A Template Manager publishes it to the catalogue.',
      actions: [{ label: 'Open Template Catalogue', to: { name: routeName } }],
    },
    global: { plugins: [pinia, router] },
  })
}

describe('workflow stage banner call to action', () => {
  it('offers the action to the role its destination admits', () => {
    const wrapper = mountBanner(['TEMPLATE_MANAGER'], ROUTES.TEMPLATE_CATALOGUES.LIST)

    expect(wrapper.text()).toContain('Open Template Catalogue')
    expect(wrapper.find('a').exists()).toBe(true)
  })

  // A TEMPLATE_CREATOR is sent to the template view legitimately and reads this
  // banner there; the catalogue route is TEMPLATE_MANAGER-only.
  it('drops the action when the route guard would bounce the reader', () => {
    const wrapper = mountBanner(['TEMPLATE_CREATOR'], ROUTES.TEMPLATE_CATALOGUES.LIST)

    expect(wrapper.text()).not.toContain('Open Template Catalogue')
    expect(wrapper.text()).toContain('A Template Manager publishes it to the catalogue.')
  })

  it('keeps actions whose destination carries no role requirement', () => {
    const wrapper = mountBanner([], ROUTES.FRONT_PAGE)

    expect(wrapper.text()).toContain('Open Template Catalogue')
  })
})
