import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, type MockInstance, vi } from 'vitest'
import BuildVersion from './BuildVersion.vue'

const getInfo = vi.hoisted(() => vi.fn())

vi.mock('@/services/info-service', () => ({
  infoService: { get: getInfo },
}))

const FRONTEND_SHA = '1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b'
const BACKEND_SHA = 'f0e1d2c3b4a5968778695a4b3c2d1e0f9a8b7c6d'

describe('build version', () => {
  let log: MockInstance<typeof console.log>

  beforeEach(() => {
    window.__DCS_VERSION__ = { frontend: { git_sha: FRONTEND_SHA } }
    log = vi.spyOn(console, 'log').mockReturnValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    delete window.__DCS_VERSION__
  })

  it('shows the short frontend and backend git SHA and logs them', async () => {
    getInfo.mockResolvedValue({ git_sha: BACKEND_SHA })

    const wrapper = mount(BuildVersion)
    await flushPromises()

    const text = wrapper.get('[data-testid="build-version"]').text()
    expect(text).toContain('Frontend 1a2b3c4')
    expect(text).toContain('Backend f0e1d2c')
    expect(text).not.toContain(FRONTEND_SHA)
    expect(log.mock.calls.some(([message]) => String(message).includes(BACKEND_SHA))).toBe(true)
  })

  it('shows the backend as unknown when /info fails', async () => {
    getInfo.mockRejectedValue(new Error('offline'))

    const wrapper = mount(BuildVersion)
    await flushPromises()

    const text = wrapper.get('[data-testid="build-version"]').text()
    expect(text).toContain('Frontend 1a2b3c4')
    expect(text).toContain('Backend unknown')
  })
})
