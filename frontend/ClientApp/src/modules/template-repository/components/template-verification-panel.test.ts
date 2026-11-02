import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import TemplateVerificationPanel from './TemplateVerificationPanel.vue'
import type { TemplateProvenanceCredential } from '@/models/responses/template-response'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-TR-09, DCS-FR-TR-20, DCS-FR-TR-24, DCS-IR-TR-05: the panel verifies the
 * template, lists its provenance credentials, checks the PDF and exports the
 * bundle, each only for the roles the endpoint accepts.
 */

const DID = 'did:web:example.com:template:1'

const verify = vi.fn()
const provenance = vi.fn()
const verifyPdf = vi.fn()
const exportBundle = vi.fn()
const downloadBlob = vi.fn()

vi.mock('@/services/contract-template-service', () => ({
  contractTemplateService: {
    verify: (request: { did: string }) => verify(request),
    provenance: (did: string) => provenance(did),
    verifyPdf: (did: string) => verifyPdf(did),
    exportBundle: (did: string) => exportBundle(did),
  },
}))
vi.mock('@/utils/download-blob', () => ({
  downloadBlob: (blob: Blob, name: string) => downloadBlob(blob, name),
}))

const credential: TemplateProvenanceCredential = {
  version: 2,
  vc_id: 'urn:uuid:vc-2',
  previous_vc_id: 'urn:uuid:vc-1',
  credential: {
    issuer: 'did:web:example.com:dcs',
    credentialSubject: {
      template_did: DID,
      version: 2,
      template_hash: 'sha256-abc',
      created_by: 'did:web:example.com:creator',
      reviewed_by: ['did:web:example.com:reviewer-a', 'did:web:example.com:reviewer-b'],
      approved_by: ['did:web:example.com:approver'],
      registered_by: 'did:web:example.com:manager',
      registrar_holder_did: 'did:web:example.com:registrar',
      previous_credential: 'urn:uuid:vc-1',
    },
  },
}

function mountAs(roles: UserRole[], props: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  return mount(TemplateVerificationPanel, { props: { did: DID, ...props }, global: { plugins: [pinia] } })
}

const buttons = (wrapper: ReturnType<typeof mountAs>) => wrapper.findAll('button').map((button) => button.text())

function button(wrapper: ReturnType<typeof mountAs>, label: string) {
  const found = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  if (!found) throw new Error(`no button ${label}`)
  return found
}

describe('TemplateVerificationPanel', () => {
  beforeEach(() => {
    verify.mockReset()
    provenance.mockReset()
    verifyPdf.mockReset()
    exportBundle.mockReset()
    downloadBlob.mockReset()
  })

  it('shows the findings and the warnings of the verification separately', async () => {
    verify.mockResolvedValue({ did: DID, findings: ['SHACL: name missing'], warnings: ['Catalogue entry is stale'] })
    const wrapper = mountAs(['TEMPLATE_APPROVER'])

    await button(wrapper, 'Verify template').trigger('click')
    await flushPromises()

    expect(verify).toHaveBeenCalledWith({ did: DID })
    expect(wrapper.get('[data-testid="template-verify-findings"]').text()).toBe('SHACL: name missing')
    expect(wrapper.get('[data-testid="template-verify-warnings"]').text()).toBe('Catalogue entry is stale')
  })

  it('lists each provenance credential with its actors and its predecessor', async () => {
    provenance.mockResolvedValue([credential])
    const wrapper = mountAs(['TEMPLATE_MANAGER'])

    await button(wrapper, 'Load provenance').trigger('click')
    await flushPromises()

    expect(provenance).toHaveBeenCalledWith(DID)
    const text = wrapper.get('[data-testid="template-provenance-credential"]').text()
    for (const value of [
      'Version 2',
      'urn:uuid:vc-2',
      'did:web:example.com:dcs',
      'sha256-abc',
      'did:web:example.com:creator',
      'did:web:example.com:reviewer-a, did:web:example.com:reviewer-b',
      'did:web:example.com:approver',
      'did:web:example.com:manager',
      'did:web:example.com:registrar',
    ]) {
      expect(text).toContain(value)
    }
    expect(wrapper.get('a[href="#provenance-urn:uuid:vc-1"]').text()).toBe('urn:uuid:vc-1')
  })

  it('verifies the PDF and exports the bundle for a Template Manager', async () => {
    verifyPdf.mockResolvedValue({ match: false, jsonld_hash: 'a', base_pdf_hash: 'b', stored_base_pdf_hash: 'c' })
    const blob = new Blob(['zip'])
    exportBundle.mockResolvedValue(blob)
    const wrapper = mountAs(['TEMPLATE_MANAGER'])

    await button(wrapper, 'Verify PDF').trigger('click')
    await button(wrapper, 'Export bundle').trigger('click')
    await flushPromises()

    expect(verifyPdf).toHaveBeenCalledWith(DID)
    expect(wrapper.get('[data-testid="template-pdf-result"]').text()).toBe('The PDF does not match the template.')
    expect(exportBundle).toHaveBeenCalledWith(DID)
    expect(downloadBlob).toHaveBeenCalledWith(blob, `template-${DID}.zip`)
  })

  it('offers a Contract Creator only verify and provenance', () => {
    expect(buttons(mountAs(['CONTRACT_CREATOR']))).toEqual(['Verify template', 'Load provenance'])
  })

  it('offers a Template Creator no verify and no PDF check', () => {
    expect(buttons(mountAs(['TEMPLATE_CREATOR']))).toEqual(['Load provenance', 'Export bundle'])
  })

  it('offers only the actions the view allows', () => {
    const wrapper = mountAs(['TEMPLATE_MANAGER', 'CONTRACT_CREATOR'], { actions: ['verify', 'provenance'] })
    expect(buttons(wrapper)).toEqual(['Verify template', 'Load provenance'])
  })

  it('shows nothing for a role without access', () => {
    expect(mountAs(['AUDITOR']).find('section').exists()).toBe(false)
  })
})
