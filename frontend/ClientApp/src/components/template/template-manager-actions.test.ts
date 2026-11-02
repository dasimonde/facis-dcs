import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { type ContractTemplateState, TemplateState } from '@/types/contract-template-state'
import TemplateManagerActions from './TemplateManagerActions.vue'
import type { PartialContractTemplate, TemplateComplianceStatus } from '@/models/contract-template/contract-template'

vi.mock('@/router/router', () => ({ ROUTES: { TEMPLATES: { LIST: 'templates' } } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/services/contract-template-service', () => ({
  contractTemplateService: { archive: vi.fn().mockResolvedValue({ did: 'x' }), register: vi.fn(), publish: vi.fn() },
}))

function mountActions(state: ContractTemplateState, compliance?: TemplateComplianceStatus) {
  setActivePinia(createPinia())
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles: ['TEMPLATE_MANAGER'] }
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
    compliance_status: compliance,
  }
  return mount(TemplateManagerActions, { props: { template }, global: { stubs: { teleport: true } } })
}

function button(state: ContractTemplateState, label: string, compliance?: TemplateComplianceStatus) {
  return mountActions(state, compliance)
    .findAll('button')
    .find((candidate) => candidate.text() === label)
}

beforeEach(() => {
  vi.clearAllMocks()
  HTMLDialogElement.prototype.showModal = vi.fn()
  HTMLDialogElement.prototype.close = vi.fn()
})

/** DCS-FR-TR-17, DCS-FR-TR-18, DCS-IR-TR-07: deprecate a live template, delete one outside the pool. */
describe('template manager archive action', () => {
  it('offers Delete for a deprecated, a draft and a rejected template', () => {
    for (const state of [TemplateState.deprecated, TemplateState.draft, TemplateState.rejected]) {
      expect(button(state, 'Delete'), state).toBeDefined()
    }
  })

  it('offers Deprecate for a registered and a published template', () => {
    expect(button(TemplateState.registered, 'Deprecate')).toBeDefined()
    expect(button(TemplateState.published, 'Deprecate')).toBeDefined()
  })

  it('offers no archive action for a template in review, approved or deleted', () => {
    for (const state of [
      TemplateState.submitted,
      TemplateState.reviewed,
      TemplateState.approved,
      TemplateState.deleted,
    ]) {
      const labels = mountActions(state)
        .findAll('button')
        .map((candidate) => candidate.text())
      expect(labels, state).not.toContain('Delete')
      expect(labels, state).not.toContain('Deprecate')
    }
  })

  it('sends the justification the manager types', async () => {
    const wrapper = mountActions(TemplateState.registered)
    await wrapper
      .findAll('button')
      .find((candidate) => candidate.text() === 'Deprecate')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="confirmation-confirm"]').attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('Replaced by version 2')
    await wrapper.get('[data-testid="confirmation-confirm"]').trigger('click')
    await flushPromises()
    expect(contractTemplateService.archive).toHaveBeenCalledWith({
      did: 'did:web:example.com:template',
      updated_at: '2026-07-31T00:00:00Z',
      justification: 'Replaced by version 2',
    })
    wrapper.unmount()
  })
})

/** DCS-IR-TR-06: only a COMPLIANT approved template can be registered. */
describe('template manager register action', () => {
  it('offers Register for a compliant approved template only', () => {
    expect(button(TemplateState.approved, 'Register', 'COMPLIANT')).toBeDefined()
    expect(button(TemplateState.approved, 'Register', 'NON_COMPLIANT')).toBeUndefined()
    expect(button(TemplateState.approved, 'Register', 'UNASSESSED')).toBeUndefined()
  })
})
