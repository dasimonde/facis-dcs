import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import ComplianceViewerView from './ComplianceViewerView.vue'
import type { SignatureViewItem } from '@/services/signature-management-service'

/**
 * DCS-FR-SM-18: each signature row in the Compliance Checks tab shows the live
 * status of the credential its signer presented, not the signature record's
 * own status. A state the backend could not establish shows its reason.
 */

const CONTRACT = { did: 'did:web:example.com:contract:1', name: 'Supply agreement', state: 'SIGNED' }

function signature(partial: Partial<SignatureViewItem>): SignatureViewItem {
  return {
    signer_did: 'did:example:signer',
    credential_type: 'AES',
    status: 'SIGNED',
    format: 'PAdES',
    credential_status: 'VALID',
    ...partial,
  }
}

const signatures = [
  signature({ signer_did: 'did:example:valid' }),
  signature({ signer_did: 'did:example:revoked', credential_status: 'REVOKED' }),
  signature({
    signer_did: 'did:example:unknown',
    credential_status: 'UNKNOWN',
    credential_status_reason: 'status list check: retrieval failed',
  }),
]

vi.mock('@/services/signature-management-service', () => ({
  signatureManagementService: {
    retrieveContracts: () => Promise.resolve([CONTRACT]),
    getSignatureView: () =>
      Promise.resolve({
        did: CONTRACT.did,
        contract_state: 'SIGNED',
        signatures,
        integrity_findings: [],
      }),
  },
}))

describe('Compliance Viewer credential status per signature', () => {
  it('renders each row from its own credential_status and shows the UNKNOWN reason', async () => {
    const wrapper = shallowMount(ComplianceViewerView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    await wrapper.get('[data-testid="compliance-contract-row"]').trigger('click')
    await flushPromises()
    const complianceTab = wrapper.findAll('[role="tab"]').find((tab) => tab.text() === 'Compliance Checks')
    await complianceTab!.trigger('click')

    const cells = wrapper.findAll('[data-testid="credential-status"]')
    expect(cells.map((cell) => cell.get('.badge').text())).toEqual(['VALID', 'REVOKED', 'UNKNOWN'])
    expect(cells[0]!.get('.badge').classes()).toContain('badge-success')
    expect(cells[1]!.get('.badge').classes()).toContain('badge-error')
    expect(cells[2]!.text()).toContain('status list check: retrieval failed')
    expect(cells[0]!.find('p').exists()).toBe(false)
  })
})
