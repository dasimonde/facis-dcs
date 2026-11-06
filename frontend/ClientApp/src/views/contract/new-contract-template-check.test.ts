import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import TemplateVerificationPanel from '@template-repository/components/TemplateVerificationPanel.vue'
import { useAuthStore } from '@/stores/auth-store'
import NewContractView from './NewContractView.vue'

/**
 * DCS-FR-TR-09, DCS-FR-TR-19, DCS-FR-TR-20: the Contract Creator verifies the
 * chosen template and reads its provenance before using it.
 */

const TEMPLATE_DID = 'did:web:example.com:template:1'

vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => ({ params: {} }),
  useRouter: () => ({ push: vi.fn() }),
  onBeforeRouteLeave: vi.fn(),
}))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieve: () => Promise.resolve({ contracts: [], review_tasks: [], approval_tasks: [], negotiation_tasks: [] }),
    retrieveApprovedTemplates: () =>
      Promise.resolve([
        {
          did: TEMPLATE_DID,
          name: 'Service agreement',
          version: 1,
          state: 'PUBLISHED',
          template_type: 'CONTRACT_TEMPLATE',
          party_type: 'MULTI_PARTY',
          created_by: 'did:web:example.com:creator',
          created_at: '2026-07-01T08:00:00Z',
          updated_at: '2026-07-01T09:00:00Z',
        },
      ]),
  },
}))

it('opens the verification and provenance panel for the chosen template', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles: ['CONTRACT_CREATOR'] }
  const wrapper = shallowMount(NewContractView, {
    global: {
      plugins: [pinia],
      stubs: { ViewContractTemplateView: { template: '<div><slot name="before-tabs" /></div>' } },
    },
  })
  await flushPromises()

  const picker = wrapper.get('select')
  ;(picker.element as HTMLSelectElement).selectedIndex = 1
  await picker.trigger('change')
  expect(wrapper.findComponent(TemplateVerificationPanel).exists()).toBe(false)

  const control = wrapper.findAll('button').find((button) => button.text() === 'Check template')!
  await control.trigger('click')

  const panel = wrapper.findComponent(TemplateVerificationPanel)
  expect(panel.props('did')).toBe(TEMPLATE_DID)
  expect(panel.props('actions')).toEqual(['verify', 'provenance'])
  expect(control.attributes('aria-expanded')).toBe('true')
})
