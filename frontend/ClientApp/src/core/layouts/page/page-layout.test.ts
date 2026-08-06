import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import PageLayout from './PageLayout.vue'

describe('page layout focus target', () => {
  it('suppresses only the programmatically focused main outline', () => {
    const wrapper = mount(PageLayout, {
      attachTo: document.body,
      slots: { default: '<button type="button">Next action</button>' },
      global: {
        plugins: [createPinia()],
        stubs: {
          PageNavBar: true,
          PageSidebar: true,
        },
      },
    })

    const main = wrapper.get('main')
    const button = wrapper.get('button')

    expect(main.attributes('tabindex')).toBe('-1')
    expect(main.classes()).toContain('outline-none')
    expect(button.classes()).not.toContain('outline-none')

    main.element.focus()
    expect(document.activeElement).toBe(main.element)

    button.element.focus()
    expect(document.activeElement).toBe(button.element)

    wrapper.unmount()
  })
})
