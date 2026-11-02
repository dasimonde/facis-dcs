import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { reactive } from 'vue'
import { buildContractDocument } from '@template-repository/store/dcsDraftStore'
import { signingWorkflow } from '@/utils/signing-workflow'
import ContractDetailsEditor from './ContractDetailsEditor.vue'
import type { Contract } from '@/models/contract/contract'

/**
 * DCS-FR-SM-13: the contract author sets the signing order of each signature
 * field and the signing deadline. Both go into the contract document that the
 * signing workflow reads.
 */
describe('signing workflow authoring', () => {
  it('writes the signing order and deadline into the contract document', async () => {
    const contract = reactive({
      did: 'did:web:a:contract:1',
      contract_version: 1,
      state: 'DRAFT',
      contract_data: {
        '@type': 'dcs:Contract',
        'dcs:signatureFields': [
          { '@type': 'dcs:SignatureField', 'dcs:signatoryName': 'did:web:a' },
          { '@type': 'dcs:SignatureField', 'dcs:signatoryName': 'did:web:b' },
        ],
      },
    }) as unknown as Contract
    const wrapper = mount(ContractDetailsEditor, { props: { contract } })

    await wrapper.get('[data-testid="contract-signing-order-did:web:b"]').setValue('1')
    await wrapper.get('[data-testid="contract-signing-order-did:web:a"]').setValue('2')
    await wrapper.get('[data-testid="contract-signing-deadline"]').setValue('2026-12-15')

    const document = buildContractDocument({
      documentId: contract.did,
      storedDocument: contract.contract_data,
      blocks: [],
      layout: [],
      contractFields: [],
      contractData: [],
      policies: [],
      semanticConditionValues: [],
    })
    expect(signingWorkflow(document).order).toEqual([
      { field: 'did:web:b', position: 1 },
      { field: 'did:web:a', position: 2 },
    ])
    expect(document['dcs:signingDeadline']).toEqual({ '@value': '2026-12-15', '@type': 'xsd:date' })

    await wrapper.get('[data-testid="contract-signing-order-did:web:a"]').setValue('')
    await wrapper.get('[data-testid="contract-signing-deadline"]').setValue('')
    expect(signingWorkflow(contract.contract_data).order).toEqual([{ field: 'did:web:b', position: 1 }])
    expect(contract.contract_data?.['dcs:signingDeadline']).toBeUndefined()
  })

  it('shows the stored order and deadline read-only', () => {
    const contract = {
      did: 'did:web:a:contract:1',
      contract_version: 1,
      state: 'SIGNED',
      contract_data: {
        '@type': 'dcs:Contract',
        'dcs:signatureFields': [{ 'dcs:signatoryName': 'did:web:a', 'dcs:signingOrder': 1 }],
        'dcs:signingDeadline': { '@value': '2026-12-15', '@type': 'xsd:date' },
      },
    } as unknown as Contract
    const wrapper = mount(ContractDetailsEditor, { props: { contract, disabled: true } })

    const order = wrapper.get<HTMLInputElement>('[data-testid="contract-signing-order-did:web:a"]')
    expect(order.element.value).toBe('1')
    expect(order.element.disabled).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-testid="contract-signing-deadline"]').element.value).toBe('2026-12-15')
  })
})
