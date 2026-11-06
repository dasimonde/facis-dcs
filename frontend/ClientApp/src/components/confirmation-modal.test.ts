import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import ConfirmationModal from './ConfirmationModal.vue'

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = vi.fn()
  HTMLDialogElement.prototype.close = vi.fn()
})

it.each([undefined, 'Summary', 'Tags'])('labels the editor %s while preserving required validation', async (label) => {
  const wrapper = mount(ConfirmationModal, { global: { stubs: { teleport: true } } })
  const result = wrapper.vm.reveal({ message: 'Edit value', editor: { requiredText: true, label } })
  await flushPromises()
  const fieldLabel = wrapper.get('label')
  expect(fieldLabel.text()).toBe(label ?? 'Comment')
  expect(fieldLabel.attributes('for')).toBe(wrapper.get('textarea').attributes('id'))
  expect(wrapper.get('[data-testid="confirmation-confirm"]').attributes('disabled')).toBeDefined()
  await wrapper.get('textarea').setValue('  ')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').attributes('disabled')).toBeDefined()
  await wrapper.get('textarea').setValue('Provided value')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').attributes('disabled')).toBeUndefined()
  await wrapper.get('dialog').trigger('cancel')
  expect((await result).isCanceled).toBe(true)
  wrapper.unmount()
})

it('keeps default titles and actions and resets editor values between reveals', async () => {
  const wrapper = mount(ConfirmationModal, { global: { stubs: { teleport: true } } })
  const first = wrapper.vm.reveal({ message: 'Confirm selection' })
  await flushPromises()
  expect(wrapper.get('h3').text()).toBe('Confirmation')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').text()).toBe('Confirm')
  await wrapper.get('[data-testid="confirmation-confirm"]').trigger('click')
  expect((await first).isCanceled).toBe(false)
  await flushPromises()
  const second = wrapper.vm.reveal({
    title: 'Edit summary – 1 of 2',
    confirmLabel: 'Next',
    message: 'Edit summary',
    editor: { requiredText: true, initialValue: 'Existing summary' },
  })
  await flushPromises()
  expect(wrapper.get('h3').text()).toBe('Edit summary – 1 of 2')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').text()).toBe('Next')
  expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Existing summary')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').attributes('disabled')).toBeUndefined()
  await wrapper.get('[data-testid="confirmation-confirm"]').trigger('click')
  expect((await second).data).toBe('Existing summary')
  await flushPromises()
  const third = wrapper.vm.reveal({ message: 'Default editor', editor: { requiredText: false } })
  await flushPromises()
  expect(wrapper.get('h3').text()).toBe('Confirmation')
  expect(wrapper.get('[data-testid="confirmation-confirm"]').text()).toBe('Submit')
  expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
  await wrapper.get('dialog').trigger('cancel')
  expect((await third).isCanceled).toBe(true)
  wrapper.unmount()
})

it('still requires acknowledgement in addition to prefilled required text', async () => {
  const wrapper = mount(ConfirmationModal, { global: { stubs: { teleport: true } } })
  const result = wrapper.vm.reveal({
    message: 'Delete entry',
    editor: { requiredText: true, initialValue: 'Existing reason' },
    acknowledgement: 'Irreversible deletion',
  })
  await flushPromises()
  const confirm = () => wrapper.get('[data-testid="confirmation-confirm"]')
  expect(confirm().attributes('disabled')).toBeDefined()
  await wrapper.get('input[type="checkbox"]').setValue(true)
  await flushPromises()
  expect(confirm().attributes('disabled')).toBeUndefined()
  await wrapper.get('textarea').setValue(' ')
  expect(confirm().attributes('disabled')).toBeDefined()
  await wrapper.get('textarea').setValue('Reason')
  await confirm().trigger('click')
  expect((await result).data).toBe('Reason')
  wrapper.unmount()
})
