import { mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { TemplateState } from '@/types/contract-template-state'
import TemplateListItem from './TemplateListItem.vue'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

/**
 * DCS-FR-TR-28, DCS-FR-TR-05: a template card shows when and by whom the
 * template last changed, and who created it.
 */
describe('template card provenance', () => {
  it('shows the last change, its actor and the creator', () => {
    setActivePinia(createPinia())
    const template = {
      did: 'did:web:example.com:template',
      name: 'Supply template',
      state: TemplateState.draft,
      template_type: 'CONTRACT_TEMPLATE',
      version: 2,
      created_by: 'did:web:example.com:creator',
      updated_by: 'did:web:example.com:approver',
      created_at: '2026-07-01T08:00:00Z',
      updated_at: '2026-07-31T09:30:00Z',
    } as PartialContractTemplate

    const text = mount(TemplateListItem, {
      props: { template },
      global: { stubs: { RouterLink: RouterLinkStub } },
    }).text()

    expect(text).toContain(`Last changed: ${new Date('2026-07-31T09:30:00Z').toLocaleString()}`)
    expect(text).toContain('Last changed by: did:web:example.com:approver')
    expect(text).toContain('Created by: did:web:example.com:creator')
  })
})
