import { flushPromises, shallowMount } from '@vue/test-utils'
import { AxiosError } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { useErrorStore } from '@/stores/error-store'
import { bindReportedHttpError } from '@/utils/report-action-error'
import ViewContractView from './ViewContractView.vue'
import type { Contract } from '@/models/contract/contract'

const retrieve = vi.hoisted(() => vi.fn())
const route = reactive({ params: { did: 'did:test:first' } })
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => route,
}))
vi.mock('@/services/did-service', () => ({ resolveLocalInstanceDid: vi.fn().mockResolvedValue('org') }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: { retrieveById: retrieve } }))
const contract: Contract = {
  did: 'did:test:first',
  name: 'Recovered contract',
  contract_version: 1,
  state: 'SIGNED',
  created_by: 'creator',
  created_at: '',
  updated_at: '',
}
function render() {
  return shallowMount(ViewContractView)
}
describe('contract loading feedback', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().user = { issuer: 'org', holder: 'manager', roles: ['CONTRACT_MANAGER'] }
    useContractsStore().contracts = [contract]
    route.params.did = contract.did
    retrieve.mockReset()
  })
  it.each(['null', 'rejection'])(
    'shows a persistent generic error for %s and retries the current DID',
    async (failure) => {
      if (failure === 'null') retrieve.mockResolvedValueOnce(null)
      else retrieve.mockRejectedValueOnce(new Error('Private backend details'))
      const wrapper = render()
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain(
        'The contract could not be loaded. You may not have access.',
      )
      expect(wrapper.text()).not.toContain('Private backend details')
      expect(wrapper.findComponent({ name: 'ContractManagerActions' }).exists()).toBe(false)
      expect(wrapper.findAll('button').filter((b) => b.text().startsWith('Export'))).toHaveLength(0)
      expect(wrapper.findComponent({ name: 'RouterLink' }).props('to')).toEqual({ name: 'contracts.list' })
      let resolve!: (value: Contract) => void
      retrieve.mockReturnValueOnce(new Promise<Contract>((res) => (resolve = res)))
      await wrapper.get('[role="alert"] button').trigger('click')
      expect(wrapper.get('[role="status"]').text()).toBe('Loading contract…')
      expect(wrapper.get('[role="alert"] button').attributes('disabled')).toBeDefined()
      expect(retrieve).toHaveBeenLastCalledWith({ did: contract.did })
      resolve(contract)
      await flushPromises()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.findAll('button').filter((b) => b.text().startsWith('Export'))).toHaveLength(2)
      expect(wrapper.findComponent({ name: 'ContractDetailsEditor' }).props('contract').did).toBe(contract.did)
      wrapper.unmount()
    },
  )
  it.each([false, true])('claims only the bound Axios toast, including stale failure: %s', async (stale) => {
    const errors = useErrorStore()
    const unrelatedId = errors.add('Unrelated failure')
    const backendError = new AxiosError('Private backend denial')
    const errorId = errors.add('Private backend denial')
    bindReportedHttpError(backendError, errorId)
    let reject!: (error: unknown) => void
    retrieve.mockReturnValueOnce(
      new Promise<Contract>((_, rej) => {
        reject = rej
      }),
    )
    const wrapper = render()
    if (stale) {
      retrieve.mockResolvedValueOnce(contract)
      route.params.did = 'did:test:next'
      await flushPromises()
    }
    reject(backendError)
    await flushPromises()
    expect(errors.errors).toEqual([{ id: unrelatedId, type: 'error', message: 'Unrelated failure' }])
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(stale ? 0 : 1)
    expect(wrapper.text()).not.toContain('Private backend denial')
    wrapper.unmount()
  })
  it.each(['resolve', 'reject'])('ignores a previous DID response that later %ss', async (outcome) => {
    let resolve!: (value: Contract) => void
    let reject!: (error: Error) => void
    retrieve.mockReturnValueOnce(
      new Promise<Contract>((res, rej) => {
        resolve = res
        reject = rej
      }),
    )
    const wrapper = render()
    retrieve.mockResolvedValueOnce(null)
    route.params.did = 'did:test:denied'
    await flushPromises()
    if (outcome === 'resolve') resolve(contract)
    else reject(new Error('Previous DID failed'))
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'ContractDetailsEditor' }).exists()).toBe(false)
    wrapper.unmount()
  })
})
