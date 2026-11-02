import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ContractPdfVerifierPanel from './ContractPdfVerifierPanel.vue'
import type { ContractPdfVerifyResult } from '@/models/responses/contract-response'
import type { UserRole } from '@/types/user-role'

/**
 * The C2PA verifier (DCS-OR-C2PA-006, -008): the banner and the four named
 * checks, for the stored PDF and for an uploaded copy.
 */

const service = vi.hoisted(() => ({ verifyPdf: vi.fn(), verifyPdfCopy: vi.fn() }))
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: service }))

const did = 'did:web:example.com:contract'

function verified(overrides: Partial<ContractPdfVerifyResult> = {}): ContractPdfVerifyResult {
  return {
    match: true,
    jsonld_hash: 'j',
    base_pdf_hash: 'b',
    stored_base_pdf_hash: 'b',
    c2pa_manifest_found: true,
    c2pa_signature_status: 'valid',
    c2pa_signer_status: 'valid',
    c2pa_signer_detail: 'urn:c2pa:1 signed by did:web:dcs-a.example',
    vc_proof_status: 'valid',
    status_list_check: 'passed',
    status_list_status: 'active',
    lifecycle_status: 'active',
    pdf_signature_status: 'valid',
    pdf_signature_source: 'eu-dss',
    pdf_signature_detail: 'signature 1 by "Signatory": INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND',
    ...overrides,
  }
}

function mountPanel(roles: UserRole[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  return mount(ContractPdfVerifierPanel, { props: { did }, global: { plugins: [pinia] } })
}

const checkRows = (wrapper: ReturnType<typeof mountPanel>) =>
  wrapper.findAll('[data-testid="pdf-verifier-check"]').map((row) => row.text())

describe('ContractPdfVerifierPanel', () => {
  it('shows the banner and the four named checks of the stored PDF', async () => {
    service.verifyPdf.mockResolvedValueOnce(verified({ lifecycle_status: 'terminated', status_list_status: 'revoked' }))
    const wrapper = mountPanel(['AUDITOR'])

    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Verify stored PDF')!
      .trigger('click')
    await flushPromises()

    expect(service.verifyPdf).toHaveBeenCalledWith(did)
    expect(wrapper.get('[data-testid="pdf-verifier-banner"]').text()).toBe('Terminated')
    const rows = checkRows(wrapper)
    expect(rows).toHaveLength(4)
    expect(rows[0]).toContain('PDF signature')
    expect(rows[0]).toContain('Valid')
    expect(rows[0]).toContain('Checked by eu-dss')
    expect(rows[1]).toContain('C2PA manifest')
    expect(rows[1]).toContain('Signers: Valid')
    expect(rows[1]).toContain('signed by did:web:dcs-a.example')
    expect(rows[2]).toContain('VC signature')
    expect(rows[3]).toContain('Status list')
    expect(rows[3]).toContain('revoked')
  })

  it('verifies an uploaded copy and reports a mismatch', async () => {
    service.verifyPdfCopy.mockResolvedValueOnce(
      verified({ match: false, discrepancy: 'content_hash_mismatch', c2pa_manifest_source: 'remote' }),
    )
    const wrapper = mountPanel(['CONTRACT_MANAGER'])
    const copy = new File(['%PDF tampered'], 'copy.pdf', { type: 'application/pdf' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [copy] })

    await input.trigger('change')
    await flushPromises()

    expect(service.verifyPdfCopy).toHaveBeenCalledWith(did, copy)
    expect(wrapper.get('[data-testid="pdf-verifier-match"]').text()).toBe('Mismatch: content_hash_mismatch')
    expect(checkRows(wrapper)[1]).toContain('remote manifest')
  })

  it('is not shown to a role the verify endpoints refuse', () => {
    const wrapper = mountPanel(['CONTRACT_SIGNER'])
    expect(wrapper.find('[data-testid="pdf-verifier"]').exists()).toBe(false)
  })
})
