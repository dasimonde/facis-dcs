import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import AuditView from './AuditView.vue'
import type { AuditFinding } from '@/models/responses/auditing-response'

const { audit, report } = vi.hoisted(() => ({ audit: vi.fn(), report: vi.fn() }))
vi.mock('@/utils/download-blob', () => ({ downloadBlob: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('@/services/archive-service', () => ({ archiveService: { erasureStatus: vi.fn().mockResolvedValue(null) } }))
vi.mock('@/services/auditing-service', () => ({ auditingService: { audit, report } }))
const finding: AuditFinding = {
  id: 'result-a',
  category: 'compliance_check',
  title: 'Contract A checked',
  status: 'PASSED',
  did: 'did:test:a',
  created_at: '2026-10-26T00:00:00Z',
}
function render() {
  return mount(AuditView, {
    global: {
      mocks: { $route: { meta: { name: 'Audit' } } },
      stubs: { ActionHistoryPanel: true, AccessAttempts: true },
    },
  })
}
function button(wrapper: ReturnType<typeof render>, text: string) {
  return wrapper.findAll('button').find((button) => button.text() === text)!
}
function expectNoResult(wrapper: ReturnType<typeof render>) {
  expect(wrapper.text()).not.toContain('Contract A checked')
  expect(wrapper.text()).not.toContain('Audit passed.')
  expect(wrapper.text()).toContain('Select a scope and execute an audit.')
  expect(wrapper.findAll('tbody tr[tabindex]')).toHaveLength(0)
  expect(wrapper.findAll('.dropdown input[type="checkbox"]')).toHaveLength(0)
  expect(wrapper.findAll('.dropdown')).toHaveLength(0)
  expect(wrapper.find('aside').exists()).toBe(false)
  expect(wrapper.get('[data-testid="audit-counters"]').text().match(/—/g)).toHaveLength(4)
  for (const format of ['JSON', 'CSV', 'PDF']) expect(button(wrapper, format).attributes('disabled')).toBeDefined()
}

describe('audit execution context', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['AUDITOR'] }
    audit.mockReset()
    report.mockReset()
  })
  it('clears findings, filter values and opened details on DID or scope changes', async () => {
    audit.mockResolvedValue([finding])
    const wrapper = render()
    await wrapper.get('#input-did').setValue('did:test:a')
    await wrapper.get('#input-justification').setValue('Review context')
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    await wrapper.get('tbody tr[tabindex]').trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('aside').text()).toContain('Contract A checked')
    await wrapper.get('#input-did').setValue('did:test:b')
    await flushPromises()
    expectNoResult(wrapper)
    await wrapper.get('#input-did').setValue('did:test:a')
    await flushPromises()
    expect(wrapper.findAll('tbody tr[tabindex]')).toHaveLength(1)
    expect(wrapper.find('aside').exists()).toBe(false)
    await wrapper.get('#select-scope').setValue('templates')
    await flushPromises()
    expectNoResult(wrapper)
    wrapper.unmount()
  })
  it.each(['did', 'scope'])('does not attribute a late response to a different %s', async (context) => {
    let resolve!: (findings: AuditFinding[]) => void
    audit.mockReturnValue(new Promise<AuditFinding[]>((res) => (resolve = res)))
    const wrapper = render()
    await wrapper.get('#input-did').setValue('did:test:a')
    await wrapper.get('#input-justification').setValue('Review context')
    await button(wrapper, 'Execute Audit').trigger('click')
    expect(button(wrapper, 'Execute Audit').text()).toBe('Execute Audit')
    expect(button(wrapper, 'Execute Audit').get('.loading').attributes('aria-hidden')).toBe('true')
    expect(wrapper.get('#input-did').attributes('disabled')).toBeDefined()
    expect(wrapper.get('#select-scope').attributes('disabled')).toBeDefined()
    // Simulate an external context change while the disabled form awaits its response.
    const state = wrapper.vm as unknown as { didFilter: string; selectedScope: string }
    if (context === 'did') state.didFilter = 'did:test:b'
    else state.selectedScope = 'templates'
    resolve([finding])
    await flushPromises()
    expectNoResult(wrapper)
    wrapper.unmount()
  })
  it.each([404, 500])('preserves findings, filters and selected details after report failure %s', async (status) => {
    audit.mockResolvedValue([finding])
    report
      .mockRejectedValueOnce(new Error(`HTTP ${status}`))
      .mockResolvedValueOnce({ bytes: '{}', contentType: 'application/json', filename: 'report.json' })
    const wrapper = render()
    await wrapper.get('#input-justification').setValue('Review report')
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    await wrapper.get('tbody tr[tabindex]').trigger('click')
    await button(wrapper, 'JSON').trigger('click')
    await flushPromises()
    expect(wrapper.get('fieldset [role="alert"]').text()).toBe('Audit report could not be generated.')
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Audit passed.')
    expect(wrapper.findAll('.dropdown')).toHaveLength(4)
    expect(wrapper.get('aside').text()).toContain('Contract A checked')
    expect(wrapper.findAll('tbody tr[tabindex]')).toHaveLength(1)
    expect(button(wrapper, 'JSON').attributes('disabled')).toBeUndefined()
    await button(wrapper, 'JSON').trigger('click')
    await flushPromises()
    expect(report).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it.each(['did', 'scope'])('does not show a late report error after %s changes', async (context) => {
    audit.mockResolvedValue([finding])
    let reject!: (error: Error) => void
    report.mockReturnValue(
      new Promise((_, no) => {
        reject = no
      }),
    )
    const wrapper = render()
    await wrapper.get('#input-justification').setValue('Review report')
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    await button(wrapper, 'JSON').trigger('click')
    expect(button(wrapper, 'JSON').text()).toBe('JSON')
    expect(wrapper.get('fieldset [role="status"]').text()).toBe('Preparing JSON report…')
    expect(wrapper.get('fieldset .loading').attributes('aria-hidden')).toBe('true')
    const state = wrapper.vm as unknown as { didFilter: string; selectedScope: string }
    if (context === 'did') state.didFilter = 'did:test:different'
    else state.selectedScope = 'templates'
    await flushPromises()
    reject(new Error('Old report failure'))
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expectNoResult(wrapper)
    wrapper.unmount()
  })
  it('distinguishes unexecuted, successfully empty and filtered empty states', async () => {
    audit.mockResolvedValueOnce([]).mockResolvedValueOnce([finding])
    const wrapper = render()
    expectNoResult(wrapper)
    await wrapper.get('#input-justification').setValue('Review empty results')
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Audit completed. No matching entries were found.')
    const counters = wrapper.get('[data-testid="audit-counters"]')
    expect(counters.text()).not.toContain('—')
    expect(counters.findAll('.font-semibold').map((counter) => counter.text())).toEqual(['0', '0', '0', '0'])
    expect(wrapper.text()).not.toContain('Select a scope and execute an audit.')
    expect(wrapper.text()).not.toContain('No findings match the selected filters.')
    expect(wrapper.findAll('.dropdown')).toHaveLength(0)
    expect(wrapper.find('aside').exists()).toBe(false)
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    await wrapper.get('.dropdown input[type="checkbox"]').setValue(false)
    expect(wrapper.text()).toContain('No findings match the selected filters.')
    expect(wrapper.text()).not.toContain('Audit completed. No matching entries were found.')
    expect(wrapper.findAll('.dropdown')).toHaveLength(4)
    wrapper.unmount()
  })
  it('clears a visible report error when the query changes', async () => {
    audit.mockResolvedValue([finding])
    report.mockRejectedValue(new Error('Report unavailable'))
    const wrapper = render()
    await wrapper.get('#input-justification').setValue('Review report')
    await button(wrapper, 'Execute Audit').trigger('click')
    await flushPromises()
    await button(wrapper, 'PDF').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Audit report could not be generated.')
    await wrapper.get('#input-did').setValue('did:test:new')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
