import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import IriPicker from './IriPicker.vue'

describe('IriPicker closed vocabulary', () => {
  it('treats an existing unknown party IRI as unselected without exposing free text or persisting a replacement', () => {
    const wrapper = mount(IriPicker, {
      props: {
        modelValue: 'https://example.invalid/custom-party',
        options: [{ value: 'urn:template#party-provider', label: 'Provider' }],
        allowCustom: false,
      },
    })

    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('option[value="__custom__"]').exists()).toBe(false)
    expect(wrapper.find('select').element.value).toBe('')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
