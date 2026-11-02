import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import WebhookAdminView from './WebhookAdminView.vue'

/**
 * DCS-NFR-SQ-08, DCS-FR-TR-22: the Integration Manager manages the webhook
 * subscriptions of the organization in the UI. The signing secret is shown
 * once, after the subscription is created.
 */

const SUB = {
  id: 'sub-1',
  event: 'contract.signed',
  callback_url: 'https://orce.example/hook',
  created_at: '2026-10-31T10:00:00Z',
}

const { list, listEvents, create, remove } = vi.hoisted(() => ({
  list: vi.fn(),
  listEvents: vi.fn(),
  create: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/services/webhook-service', () => ({
  webhookService: { list, listEvents, create, remove },
}))

const reveal = vi.fn(() => Promise.resolve({ isCanceled: false }))

function mountView() {
  return mount(WebhookAdminView, {
    global: { stubs: { ConfirmationModal: { template: '<div />', methods: { reveal } } } },
  })
}

describe('Webhooks view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listEvents.mockResolvedValue([{ name: 'contract.signed', description: 'A contract was signed' }])
    list.mockResolvedValue([SUB])
  })

  it('lists the subscriptions of the organization and the events to choose from', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('[data-testid="webhook-row"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="webhook-row-event"]').text()).toBe('contract.signed')
    expect(wrapper.get('[data-testid="webhook-event"]').text()).toContain('contract.signed')
  })

  it('creates a subscription and shows its secret once', async () => {
    list.mockResolvedValueOnce([])
    create.mockResolvedValue({ ...SUB, secret: 'whsec_c2VjcmV0' })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="webhook-event"]').setValue('contract.signed')
    await wrapper.get('[data-testid="webhook-url"]').setValue(' https://orce.example/hook ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith('contract.signed', 'https://orce.example/hook')
    expect((wrapper.get('[data-testid="webhook-secret-value"]').element as HTMLInputElement).value).toBe(
      'whsec_c2VjcmV0',
    )
    expect(wrapper.findAll('[data-testid="webhook-row"]')).toHaveLength(1)

    await wrapper.get('[data-testid="webhook-secret-done"]').trigger('click')
    expect(wrapper.find('[data-testid="webhook-secret"]').exists()).toBe(false)
  })

  it('deletes a subscription after confirmation', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="webhook-delete"]').trigger('click')
    await flushPromises()
    expect(reveal).toHaveBeenCalled()
    expect(remove).toHaveBeenCalledWith('sub-1')
    expect(wrapper.find('[data-testid="webhook-row"]').exists()).toBe(false)
  })

  it('shows the message of a refused request', async () => {
    create.mockRejectedValue(
      Object.assign(new Error('Request failed with status code 400'), {
        isAxiosError: true,
        response: { data: { error: 'unknown event: contract.x' } },
      }),
    )
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="webhook-event"]').setValue('contract.signed')
    await wrapper.get('[data-testid="webhook-url"]').setValue('https://orce.example/hook')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="webhook-admin-error"]').text()).toBe('unknown event: contract.x')
  })
})
