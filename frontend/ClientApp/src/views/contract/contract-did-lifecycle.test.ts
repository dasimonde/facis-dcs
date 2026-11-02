import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import ViewContractView from './ViewContractView.vue'
import type { Contract } from '@/models/contract/contract'

const retrieve = vi.hoisted(() => vi.fn())
const route = reactive({ params: { did: 'did:test:first' } })
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => route,
}))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: { retrieveById: retrieve } }))
const original: Contract = {
  did: 'did:test:first',
  name: 'Original contract',
  contract_version: 1,
  state: 'SIGNED',
  created_by: 'creator',
  created_at: '',
  updated_at: '',
}

it('changes the actual contract DID, clears the old audit panel, and ignores late previous-contract responses', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'manager', roles: ['CONTRACT_MANAGER'] }
  useContractsStore().contracts = [original]
  retrieve.mockResolvedValueOnce(original)
  const wrapper = shallowMount(ViewContractView, { global: { plugins: [pinia] } })
  await flushPromises()
  expect(wrapper.findComponent({ name: 'AuditView' }).props('contract').did).toBe(original.did)
  expect(wrapper.findAll('[role="tab"]').some((tab) => tab.text() === 'Audit History')).toBe(true)
  let resolve!: (value: Contract) => void
  retrieve.mockReturnValueOnce(
    new Promise<Contract>((res) => {
      resolve = res
    }),
  )
  route.params.did = 'did:test:second'
  await flushPromises()
  expect(wrapper.findComponent({ name: 'AuditView' }).exists()).toBe(false)
  retrieve.mockResolvedValueOnce({ ...original, did: 'did:test:third', name: 'Third contract' })
  route.params.did = 'did:test:third'
  await flushPromises()
  expect(retrieve).toHaveBeenCalledWith({ did: 'did:test:third' })
  resolve({ ...original, did: 'did:test:second' })
  await flushPromises()
  expect(wrapper.findComponent({ name: 'AuditView' }).props('contract').did).toBe('did:test:third')
  wrapper.unmount()
})
