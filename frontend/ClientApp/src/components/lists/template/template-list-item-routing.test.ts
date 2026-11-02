import { mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState } from '@/types/contract-template-state'
import TemplateListItem from './TemplateListItem.vue'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'
import type { UserRole } from '@/types/user-role'

/**
 * Where the row's View button goes. The two role checks behind it read a
 * ComputedRef without .value, which is truthy whatever the roles are, so every
 * role landed on the Review or Approve page with the actions greyed out.
 */

function viewTarget(roles: UserRole[], state: PartialContractTemplate['state']) {
  setActivePinia(createPinia())
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  const template: PartialContractTemplate = {
    did: 'did:web:example.com:template',
    name: 'Supply template',
    state,
    template_type: 'CONTRACT_TEMPLATE',
    party_type: 'SINGLE_PARTY',
    version: 1,
    created_by: 'did:web:example.com:creator',
    created_at: '2026-07-31T00:00:00Z',
    updated_at: '2026-07-31T00:00:00Z',
  }
  const wrapper = mount(TemplateListItem, {
    props: { template },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
  const link = wrapper.findAllComponents(RouterLinkStub).find((candidate) => candidate.text().includes('View'))
  return (link?.props('to') as { name: string } | undefined)?.name
}

describe('template row View destination', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('sends a reviewer to the review page for a submitted template', () => {
    expect(viewTarget(['TEMPLATE_REVIEWER'], TemplateState.submitted)).toBe(ROUTES.TEMPLATES.REVIEW)
  })

  it('sends an approver to the approve page for a reviewed template', () => {
    expect(viewTarget(['TEMPLATE_APPROVER'], TemplateState.reviewed)).toBe(ROUTES.TEMPLATES.APPROVE)
  })

  // DCS-FR-TR-06: the Template Manager neither reviews nor approves.
  it('sends a manager to the read-only view of a submitted or reviewed template', () => {
    expect(viewTarget(['TEMPLATE_MANAGER'], TemplateState.submitted)).toBe(ROUTES.TEMPLATES.VIEW)
    expect(viewTarget(['TEMPLATE_MANAGER'], TemplateState.reviewed)).toBe(ROUTES.TEMPLATES.VIEW)
  })

  it('sends a creator to the read-only view instead of a page it cannot act on', () => {
    expect(viewTarget(['TEMPLATE_CREATOR'], TemplateState.submitted)).toBe(ROUTES.TEMPLATES.VIEW)
    expect(viewTarget(['TEMPLATE_CREATOR'], TemplateState.reviewed)).toBe(ROUTES.TEMPLATES.VIEW)
  })
})
