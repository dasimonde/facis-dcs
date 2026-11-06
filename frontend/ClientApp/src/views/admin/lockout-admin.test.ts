import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LockoutAdminView from './LockoutAdminView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), unlock: vi.fn() }))
vi.mock('@/services/lockout-service', () => ({ lockoutService: { list: mocks.list, unlock: mocks.unlock } }))

beforeEach(() => {
  vi.resetAllMocks()
})

// NFR-18: the System Administrator sees the active lockouts and can end one.
describe('lockout admin view', () => {
  it('lists the active lockouts and ends one on unlock', async () => {
    mocks.list
      .mockResolvedValueOnce([
        { key: '203.0.113.9', locked_until: '2026-10-31T12:00:00Z' },
        { key: 'holder:did:jwk:abc', locked_until: '2026-10-31T12:05:00Z' },
      ])
      .mockResolvedValueOnce([{ key: 'holder:did:jwk:abc', locked_until: '2026-10-31T12:05:00Z' }])
    mocks.unlock.mockResolvedValue(undefined)

    const wrapper = mount(LockoutAdminView)
    await flushPromises()
    expect(wrapper.findAll('[data-testid="lockout-row"]')).toHaveLength(2)

    await wrapper.get('[aria-label="Unlock 203.0.113.9"]').trigger('click')
    await flushPromises()
    expect(mocks.unlock).toHaveBeenCalledWith('203.0.113.9')
    expect(wrapper.findAll('[data-testid="lockout-row"]')).toHaveLength(1)
  })

  it('says when nothing is locked and shows a failed unlock', async () => {
    mocks.list.mockResolvedValue([{ key: '203.0.113.9', locked_until: '2026-10-31T12:00:00Z' }])
    mocks.unlock.mockRejectedValue(new Error('no active lockout for this key'))
    const wrapper = mount(LockoutAdminView)
    await flushPromises()
    await wrapper.get('[data-testid="lockout-unlock"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="lockouts-error"]').text()).toContain('no active lockout for this key')

    mocks.list.mockResolvedValue([])
    const empty = mount(LockoutAdminView)
    await flushPromises()
    expect(empty.find('[data-testid="lockouts-empty-state"]').exists()).toBe(true)
  })
})
