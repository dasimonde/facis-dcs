import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import {
  applyInlineSemanticValues,
  fromDocumentSemanticValues,
} from '@contract-workflow-engine/utils/semantic-condition-values'
import PreviewParamInput from './PreviewParamInput.vue'
import type { DcsContractField } from '@/models/dcs-jsonld'

vi.mock('@template-repository/utils/value-option-catalog', () => ({ resolveValueOptions: () => [] }))

describe('numeric field precision', () => {
  it.each([
    ['integer', 'xsd:integer', '18446744073709551615'],
    ['decimal', 'xsd:decimal', '12345678901234567890.123456789'],
  ] as const)('preserves a %s value through editing and reload', async (type, datatype, lexical) => {
    const wrapper = mount(PreviewParamInput, { props: { type } })
    await wrapper.get('input').setValue(lexical)
    const emitted = wrapper.emitted('update:value') ?? []
    const value = emitted[emitted.length - 1]?.[0]
    expect(value).toBe(lexical)
    const field = {
      '@id': 'urn:field',
      '@type': 'dcs:ContractField',
      'dcs:label': 'Amount',
      'dcs:datatype': datatype,
    } as DcsContractField
    const saved = applyInlineSemanticValues(
      [field],
      [{ blockId: '', conditionId: 'urn:field', parameterName: 'Amount', parameterValue: value as string }],
    )
    expect(saved[0]?.['dcs:value']).toEqual({ '@type': datatype, '@value': lexical })
    expect(fromDocumentSemanticValues(JSON.parse(JSON.stringify(saved)))[0]?.parameterValue).toBe(lexical)
    wrapper.unmount()
  })
})
