import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import AuditView from './AuditView.vue'
import type { UserRole } from '@/types/user-role'

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/services/archive-service', () => ({ archiveService: { erasureStatus: vi.fn().mockResolvedValue(null) } }))
vi.mock('@/services/auditing-service', () => ({ auditingService: {} }))

it.each<[UserRole, boolean, boolean]>([
  ['AUDITOR', true, true],
  ['COMPLIANCE_OFFICER', false, true],
  ['ARCHIVE_MANAGER', true, false],
])('integrates compliance, access attempts and history for %s', async (role, compliance, history) => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: [role] }
  const wrapper = mount(AuditView, {
    global: {
      plugins: [pinia],
      mocks: { $route: { meta: { name: 'Audit' } } },
      stubs: { ActionHistoryPanel: true },
    },
  })
  await flushPromises()
  expect(
    wrapper.findComponent({ name: 'AccessAttempts' }).exists() &&
      wrapper.findComponent({ name: 'AccessAttempts' }).find('details').exists(),
  ).toBe(role === 'AUDITOR')
  expect(wrapper.find('#select-scope').exists()).toBe(compliance)
  const historyTab = wrapper.findAll('[role="tab"]').find((tab) => tab.text() === 'Action History')
  if (historyTab) await historyTab.trigger('click')
  expect(wrapper.findComponent({ name: 'ActionHistoryPanel' }).exists()).toBe(history)
  wrapper.unmount()
})

// DCS-NFR-SEC-04: the Auditor reads the configuration attestation records of
// the backend starts, which the PROCESS_AUDIT_AND_COMPLIANCE scope returns.
it('offers the Auditor the configuration attestation scope', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['AUDITOR'] }
  const wrapper = mount(AuditView, {
    global: { plugins: [pinia], mocks: { $route: { meta: { name: 'Audit' } } }, stubs: { ActionHistoryPanel: true } },
  })
  await flushPromises()
  const values = wrapper
    .get('select')
    .findAll('option')
    .map((option) => option.attributes('value'))
  expect(values).toContain('PROCESS_AUDIT_AND_COMPLIANCE')
  wrapper.unmount()
})
