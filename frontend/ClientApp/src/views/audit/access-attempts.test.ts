import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccessAttempts from './AccessAttempts.vue'
import type { AccessAttempt, AccessAttemptsResponse } from '@/models/responses/auditing-response'

const mocks = vi.hoisted(() => ({
  auth: { user: { roles: ['AUDITOR'] as string[] } },
  accessAttempts: vi.fn(),
  claimError: vi.fn(),
}))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/services/auditing-service', () => ({ auditingService: { accessAttempts: mocks.accessAttempts } }))
vi.mock('@/utils/report-action-error', () => ({ claimReportedHttpError: mocks.claimError }))

const attempt: AccessAttempt = {
  id: 7,
  attempted_at: '2026-10-15T12:00:00Z',
  attempt_by: '<img src=x onerror=alert(1)>',
  roles: ['ARCHIVE_MANAGER'],
  service: 'ProcessAuditAndCompliance',
  method: 'Audit',
  scope: 'contracts',
  did: 'did:web:example:contract',
  justification: 'Review access',
  outcome: 'DENIED',
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.auth.user.roles = ['AUDITOR']
  mocks.accessAttempts.mockResolvedValue({ attempts: [] })
})

async function load(wrapper: ReturnType<typeof mount>, purpose = '  Investigate denials  ') {
  await wrapper.get('input').setValue(purpose)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}

describe('access attempt audit view', () => {
  it.each([
    { roles: [] },
    { roles: ['ARCHIVE_MANAGER'] },
    { roles: ['COMPLIANCE_OFFICER'] },
    { roles: ['SYSTEM_ADMINISTRATOR'] },
  ])('does not expose access logs for roles $roles', ({ roles }) => {
    mocks.auth.user.roles = roles
    const wrapper = mount(AccessAttempts)
    expect(wrapper.find('details').exists()).toBe(false)
    expect(mocks.accessAttempts).not.toHaveBeenCalled()
  })

  it('requires explicit loading and a nonblank purpose, including mixed Auditor roles', async () => {
    mocks.auth.user.roles = ['ARCHIVE_MANAGER', 'AUDITOR']
    const wrapper = mount(AccessAttempts)
    expect(wrapper.text()).toContain('Enter a justification')
    expect(mocks.accessAttempts).not.toHaveBeenCalled()
    expect(wrapper.get('label').attributes('for')).toBe(wrapper.get('input').attributes('id'))
    expect(wrapper.get('input').attributes('required')).toBeDefined()
    await load(wrapper, '   ')
    expect(mocks.accessAttempts).not.toHaveBeenCalled()
    await load(wrapper)
    expect(mocks.accessAttempts).toHaveBeenCalledExactlyOnceWith({
      justification: 'Investigate denials',
      cursor: undefined,
    })
    expect(wrapper.get('[role="status"]').text()).toContain('No access attempts found')
  })

  it('announces loading and prevents simultaneous requests', async () => {
    let resolve!: (response: AccessAttemptsResponse) => void
    mocks.accessAttempts.mockReturnValue(
      new Promise<AccessAttemptsResponse>((done) => {
        resolve = done
      }),
    )
    const wrapper = mount(AccessAttempts)
    await wrapper.get('input').setValue('Investigate')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="status"]').text()).toContain('Loading')
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(mocks.accessAttempts).toHaveBeenCalledTimes(1)
    resolve({ attempts: [] })
    await flushPromises()
    expect(wrapper.get('fieldset').attributes('disabled')).toBeUndefined()
  })

  it('renders denial metadata as text and keeps the table keyboard reachable', async () => {
    mocks.accessAttempts.mockResolvedValue({ attempts: [attempt] })
    const wrapper = mount(AccessAttempts)
    await load(wrapper)
    const row = wrapper.get('tbody tr')
    for (const value of [
      attempt.attempt_by!,
      ...attempt.roles,
      attempt.service,
      attempt.method,
      attempt.scope!,
      attempt.did!,
      attempt.justification,
      'DENIED',
    ]) {
      expect(row.text()).toContain(value)
    }
    const timestamp = row.get('time')
    expect(timestamp.attributes('datetime')).toBe(attempt.attempted_at)
    expect(timestamp.attributes('title')).toBe(attempt.attempted_at)
    expect(timestamp.text()).toBe(new Date(attempt.attempted_at).toLocaleString(undefined, { timeZone: 'UTC' }))
    expect(timestamp.text()).not.toBe(attempt.attempted_at)
    expect(row.get('summary').attributes('title')).toBe(attempt.attempt_by)
    expect(row.get('summary').text()).toContain('…')
    expect(row.get('details div').text()).toBe(attempt.attempt_by)
    expect(row.find('img').exists()).toBe(false)
    expect(row.get('.badge-error').text()).toBe('DENIED')
    expect(wrapper.findAll('th')[0]?.text()).toBe('Outcome')
    expect(row.findAll('td')[0]?.text()).toBe('DENIED')
    expect(wrapper.get('[role="region"]').attributes('tabindex')).toBe('0')
    expect(wrapper.get('[role="region"]').classes()).toContain('max-h-96')
    expect(wrapper.get('[role="region"]').classes()).toContain('overflow-auto')
  })

  it('uses opaque cursor history and the original purpose for paging, resetting on a fresh load', async () => {
    mocks.accessAttempts
      .mockResolvedValueOnce({ attempts: [attempt], next_cursor: 'opaque:first' })
      .mockResolvedValueOnce({ attempts: [{ ...attempt, id: 8 }], next_cursor: 'opaque:second' })
      .mockResolvedValueOnce({ attempts: [attempt], next_cursor: 'opaque:first' })
      .mockResolvedValueOnce({ attempts: [] })
    const wrapper = mount(AccessAttempts)
    await load(wrapper)
    expect(wrapper.get('[aria-label="Go to previous page"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input').setValue('Changed purpose')
    await wrapper.get('[aria-label="Go to next page"]').trigger('click')
    await flushPromises()
    expect(mocks.accessAttempts).toHaveBeenLastCalledWith({
      justification: 'Investigate denials',
      cursor: 'opaque:first',
    })
    expect(wrapper.get('[aria-current="page"]').text()).toBe('2')
    await wrapper.get('[aria-label="Go to previous page"]').trigger('click')
    await flushPromises()
    expect(mocks.accessAttempts).toHaveBeenLastCalledWith({ justification: 'Investigate denials', cursor: undefined })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.accessAttempts).toHaveBeenLastCalledWith({ justification: 'Changed purpose', cursor: undefined })
    expect(wrapper.get('[aria-current="page"]').text()).toBe('1')
    expect(wrapper.get('[aria-label="Go to next page"]').attributes('disabled')).toBeDefined()
  })

  it('clears stale rows after failure, claims the existing error once, and allows retry', async () => {
    const failure = new Error('Forbidden')
    mocks.accessAttempts.mockResolvedValueOnce({ attempts: [attempt] }).mockRejectedValueOnce(failure)
    const wrapper = mount(AccessAttempts)
    await load(wrapper)
    await load(wrapper)
    expect(wrapper.find('tbody').exists()).toBe(false)
    expect(wrapper.find('[aria-label="Access attempts pages"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('could not be loaded')
    expect(mocks.claimError).toHaveBeenCalledExactlyOnceWith(failure)
    expect(wrapper.get('[role="status"]').text()).not.toContain('No access attempts')
    await load(wrapper)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toContain('No access attempts found')
  })
})

it.each(['2026-10-15T12:00:00+02:00', 'invalid timestamp'])(
  'preserves original timestamp %s and safely formats UTC',
  async (timestamp) => {
    mocks.accessAttempts.mockResolvedValue({
      attempts: [{ ...attempt, attempt_by: 'short-actor', attempted_at: timestamp }],
    })
    const wrapper = mount(AccessAttempts)
    await load(wrapper)
    const time = wrapper.get('time')
    expect(time.attributes('datetime')).toBe(timestamp)
    expect(time.attributes('title')).toBe(timestamp)
    const date = new Date(timestamp)
    expect(time.text()).toBe(
      Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString(undefined, { timeZone: 'UTC' }),
    )
    expect(wrapper.get('tbody tr').text()).toContain('short-actor')
    expect(wrapper.find('tbody details').exists()).toBe(false)
    wrapper.unmount()
  },
)
