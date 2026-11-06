import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import SecureContractViewerView from './SecureContractViewerView.vue'
import type { SignatureFieldStatus } from '@/services/signature-management-service'

/**
 * DCS-FR-SM-07/-17: several signers on one instance each sign their own
 * declared field. DCS-IR-SM-04: the viewer reports a contract as executed only
 * when the validation passed and every field is signed.
 */

const CONTRACT_DID = 'did:web:example.com:contract:1'
const contract = { did: CONTRACT_DID, name: 'Supply agreement', state: 'SIGNED', contract_data: null }
const signedEnvelope = { status: 'SIGNED', signer_did: 'did:jwk:first' }

const service = vi.hoisted(() => ({
  retrieveById: vi.fn(),
  getProvenanceChain: vi.fn(() => Promise.resolve({ chain: [] })),
  verifySignature: vi.fn(() => Promise.resolve({ match: true, findings: [] })),
  validateSignature: vi.fn(),
  publishSignatureRequest: vi.fn(),
}))

vi.mock('@/services/signature-management-service', () => ({
  signatureManagementService: service,
  signingErrorMessage: (e: unknown) => String(e),
}))
vi.mock('@/services/did-service', () => ({ getLocalDIDFile: () => Promise.resolve({ id: 'did:web:org' }) }))
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ params: { did: CONTRACT_DID }, meta: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const reveal = vi.fn()
const DialogStub = defineComponent({
  name: 'SigningCeremonyDialog',
  setup(_, { expose }) {
    expose({ reveal })
    return () => null
  },
})

beforeEach(() => {
  setActivePinia(createPinia())
  useAuthStore().user = { holder: 'did:example:holder', issuer: 'did:web:org', roles: ['CONTRACT_SIGNER'] }
  vi.clearAllMocks()
})

const field = (name: string, status: 'SIGNED' | 'UNSIGNED', remote = false): SignatureFieldStatus => ({
  field_name: name,
  status,
  remote,
})

async function openViewer(fields: SignatureFieldStatus[]) {
  service.retrieveById.mockResolvedValue({ contract, signature_envelope: signedEnvelope, signature_fields: fields })
  const wrapper = shallowMount(SecureContractViewerView, {
    global: { stubs: { SigningCeremonyDialog: DialogStub } },
  })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((b) => b.text() === 'Verify')!
    .trigger('click')
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof openViewer>>, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === label)
  expect(found, label).toBeDefined()
  return found!
}

describe('Secure Contract Viewer with several signers on one instance', () => {
  it('a second signer can sign the remaining field after the first signature', async () => {
    reveal.mockResolvedValue({ isCanceled: true })
    const wrapper = await openViewer([
      field('SignerOne', 'SIGNED'),
      field('SignerTwo', 'UNSIGNED'),
      field('SignerThree', 'UNSIGNED'),
    ])

    const picker = wrapper.get('[data-testid="signature-field-picker"]')
    expect(picker.findAll('option').map((o) => o.text())).toEqual(['SignerTwo', 'SignerThree'])
    await picker.setValue('SignerThree')

    const sign = button(wrapper, 'Sign with wallet')
    expect(sign.attributes('disabled')).toBeUndefined()
    await sign.trigger('click')
    await flushPromises()

    expect(reveal).toHaveBeenCalledWith({ contractDid: CONTRACT_DID, fieldName: 'SignerThree' })
  })

  it('offers no signing when only the counterparty field is unsigned', async () => {
    const wrapper = await openViewer([field('did:web:org', 'SIGNED'), field('did:web:peer', 'UNSIGNED', true)])

    expect(wrapper.find('[data-testid="signature-field-picker"]').exists()).toBe(false)
    expect(button(wrapper, 'Sign with wallet').attributes('disabled')).toBeDefined()
  })
})

describe('Secure Contract Viewer executed banner (DCS-IR-SM-04)', () => {
  const BANNER = 'Executed contract submitted. All required signatures are applied and validated.'

  async function validateWith(fields: SignatureFieldStatus[], result: unknown) {
    service.validateSignature.mockResolvedValue(result)
    const wrapper = await openViewer(fields)
    await button(wrapper, 'Validate').trigger('click')
    await flushPromises()
    return wrapper
  }

  it('validation with findings does not show the executed banner', async () => {
    const wrapper = await validateWith([field('SignerOne', 'SIGNED')], {
      findings: ['Integrity check failed: re-render mismatch'],
      dss: { indication: 'INDETERMINATE' },
    })

    expect(wrapper.text()).not.toContain(BANNER)
    expect(wrapper.get('[role="alert"]').text()).toContain('Integrity check failed: re-render mismatch')
  })

  it('a failing DSS indication does not show the executed banner', async () => {
    const wrapper = await validateWith([field('SignerOne', 'SIGNED')], {
      findings: ['Document integrity check passed'],
      dss: { indication: 'TOTAL-FAILED' },
    })

    expect(wrapper.text()).not.toContain(BANNER)
    expect(wrapper.get('[role="alert"]').text()).toContain('TOTAL-FAILED')
  })

  it('a passed validation with an unsigned field does not show the executed banner', async () => {
    const wrapper = await validateWith([field('SignerOne', 'SIGNED'), field('SignerTwo', 'UNSIGNED')], {
      findings: ['Document integrity check passed'],
    })

    expect(wrapper.text()).not.toContain(BANNER)
    expect(wrapper.get('[data-testid="pending-fields"]').text()).toContain('SignerTwo')
  })

  it('a passed validation of a fully signed contract shows the executed banner', async () => {
    const wrapper = await validateWith([field('SignerOne', 'SIGNED'), field('SignerTwo', 'SIGNED')], {
      findings: ['Document integrity check passed'],
      dss: { indication: 'TOTAL-PASSED' },
    })

    expect(wrapper.text()).toContain(BANNER)
  })
})
