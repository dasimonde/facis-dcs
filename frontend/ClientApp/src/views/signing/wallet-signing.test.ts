import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import SigningCeremonyDialog from '@/components/signing/SigningCeremonyDialog.vue'
import { useAuthStore } from '@/stores/auth-store'
import SecureContractViewerView from './SecureContractViewerView.vue'

/**
 * DCS-FR-SM-06/-23: after the PID ceremony, the signer signs in the wallet. The
 * viewer publishes the OID4VP Document-Retrieval request, shows its deep link
 * and waits until the DCS accepted the signature the wallet posted back.
 */

const CONTRACT_DID = 'did:web:example.com:contract:1'
const WALLET_URI = 'openid4vp://?client_id=x509_san_dns%3Adcs&request_uri=https%3A%2F%2Fdcs%2Fobject'

const service = vi.hoisted(() => ({
  retrieveById: vi.fn(),
  getProvenanceChain: vi.fn((): Promise<{ chain: unknown[] }> => Promise.resolve({ chain: [] })),
  verifySignature: vi.fn(() => Promise.resolve({ match: true, findings: [] })),
  publishSignatureRequest: vi.fn(),
  startCeremony: vi.fn(),
  getCeremonyStatus: vi.fn(),
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

describe('Secure Contract Viewer wallet signing', () => {
  it('publishes the signing request and records the signature the wallet posted back', async () => {
    const contract = { did: CONTRACT_DID, name: 'Supply agreement', state: 'APPROVED', contract_data: null }
    service.retrieveById
      .mockResolvedValueOnce({ contract })
      .mockResolvedValueOnce({ contract, signature_envelope: { status: 'SIGNED', signer_did: 'did:jwk:signer' } })
    service.publishSignatureRequest.mockResolvedValue({ ceremony_id: 'ceremony-1', wallet_uri: WALLET_URI })
    reveal
      .mockResolvedValueOnce({ isCanceled: false, data: { signerDid: 'did:jwk:signer', ceremonyId: 'ceremony-1' } })
      .mockResolvedValueOnce({ isCanceled: false, data: { signerDid: 'did:jwk:signer', ceremonyId: 'ceremony-1' } })

    const wrapper = shallowMount(SecureContractViewerView, {
      global: { stubs: { SigningCeremonyDialog: DialogStub } },
    })
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Verify')!
      .trigger('click')
    await flushPromises()
    const sign = wrapper.findAll('button').find((b) => b.text() === 'Sign with wallet')
    expect(sign, 'Sign with wallet button').toBeDefined()
    await sign!.trigger('click')
    await flushPromises()

    expect(service.publishSignatureRequest).toHaveBeenCalledWith('ceremony-1')
    expect(reveal).toHaveBeenLastCalledWith(
      expect.objectContaining({ signingRequest: { ceremonyId: 'ceremony-1', walletUri: WALLET_URI } }),
    )
    expect(service.retrieveById).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Signed document accepted and recorded.')
  })
})

describe('Secure Contract Viewer signing workflow', () => {
  it('shows the signing order with the own slot and a passed signing deadline (DCS-FR-SM-13)', async () => {
    service.retrieveById.mockResolvedValueOnce({
      contract: {
        did: CONTRACT_DID,
        name: 'Supply agreement',
        state: 'APPROVED',
        contract_data: {
          'dcs:signatureFields': [
            { 'dcs:signatoryName': 'did:web:org', 'dcs:signingOrder': 2 },
            { 'dcs:signatoryName': 'did:web:peer', 'dcs:signingOrder': 1 },
          ],
          'dcs:signingDeadline': { '@value': '2026-01-01T00:00:00Z', '@type': 'xsd:dateTime' },
        },
      },
    })
    const wrapper = shallowMount(SecureContractViewerView, {
      global: { stubs: { SigningCeremonyDialog: DialogStub } },
    })
    await flushPromises()

    const panel = wrapper.get('[data-testid="signing-workflow"]')
    expect(panel.text()).toContain('Deadline passed')
    const entries = panel.findAll('li').map((li) => li.text())
    expect(entries[0]).toContain('did:web:peer')
    expect(entries[1]).toContain('did:web:org')
    expect(entries[1]).toContain('Your signature')
  })
})

describe('Signing ceremony dialog for a published signing request', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    HTMLDialogElement.prototype.showModal = vi.fn()
    HTMLDialogElement.prototype.close = vi.fn()
  })
  afterEach(() => vi.useRealTimers())

  it('shows the deep link and resolves once the wallet signature is accepted', async () => {
    service.getCeremonyStatus
      .mockResolvedValueOnce({ ceremony_id: 'ceremony-1', status: 'verified' })
      .mockResolvedValueOnce({ ceremony_id: 'ceremony-1', status: 'verified', signed_at: '2026-10-31T10:00:00Z' })
    const wrapper = mount(SigningCeremonyDialog, { attachTo: document.body })
    const outcome = wrapper.vm.reveal({
      contractDid: CONTRACT_DID,
      fieldName: 'did:web:org',
      signingRequest: { ceremonyId: 'ceremony-1', walletUri: WALLET_URI },
    })
    await flushPromises()
    expect(service.startCeremony).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('Waiting for the wallet signature')

    await vi.advanceTimersByTimeAsync(2500)
    expect(document.body.textContent).toContain('Waiting for the wallet signature')
    await vi.advanceTimersByTimeAsync(2500)

    await expect(outcome).resolves.toMatchObject({ isCanceled: false, data: { ceremonyId: 'ceremony-1' } })
    wrapper.unmount()
  })
})

async function openWalletSigning() {
  const wrapper = shallowMount(SecureContractViewerView, {
    global: { stubs: { SigningCeremonyDialog: DialogStub } },
  })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((b) => b.text() === 'Verify')!
    .trigger('click')
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((b) => b.text() === 'Sign with wallet')!
    .trigger('click')
  await flushPromises()
  return wrapper
}

const presented = (ceremonyId: string) => ({ isCanceled: false, data: { signerDid: 'did:jwk:signer', ceremonyId } })

describe('Secure Contract Viewer wallet signing retries (DCS-FR-SM-23)', () => {
  const contract = { did: CONTRACT_DID, name: 'Supply agreement', state: 'APPROVED', contract_data: null }

  it('retry after an expired signing request publishes a new request', async () => {
    service.retrieveById.mockResolvedValue({ contract })
    service.publishSignatureRequest.mockResolvedValue({ ceremony_id: 'ceremony-1', wallet_uri: WALLET_URI })
    reveal
      .mockResolvedValueOnce(presented('ceremony-1'))
      .mockResolvedValueOnce({ isCanceled: false, data: { signerDid: '', ceremonyId: 'ceremony-1', republish: true } })
      .mockResolvedValueOnce(presented('ceremony-1'))

    await openWalletSigning()

    expect(service.publishSignatureRequest).toHaveBeenCalledTimes(2)
    expect(reveal).toHaveBeenCalledTimes(3)
    expect(reveal.mock.calls[2]![0]).toMatchObject({ signingRequest: { ceremonyId: 'ceremony-1' } })
  })

  it('starts a new ceremony when the verified ceremony can no longer be published', async () => {
    service.retrieveById.mockResolvedValue({ contract })
    service.publishSignatureRequest
      .mockRejectedValueOnce({ response: { data: { name: 'ceremony_required' } } })
      .mockResolvedValueOnce({ ceremony_id: 'ceremony-2', wallet_uri: WALLET_URI })
    reveal
      .mockResolvedValueOnce(presented('ceremony-1'))
      .mockResolvedValueOnce(presented('ceremony-2'))
      .mockResolvedValueOnce(presented('ceremony-2'))

    const wrapper = await openWalletSigning()

    expect(service.publishSignatureRequest.mock.calls).toEqual([['ceremony-1'], ['ceremony-2']])
    expect(reveal.mock.calls[1]![0]).not.toHaveProperty('signingRequest')
    expect(wrapper.text()).not.toContain('Wallet signing failed')
  })

  it('reports an accepted signature as recorded when the reload fails (DCS-FR-SM-09)', async () => {
    service.retrieveById.mockResolvedValueOnce({ contract }).mockRejectedValueOnce(new Error('gateway timeout'))
    service.publishSignatureRequest.mockResolvedValue({ ceremony_id: 'ceremony-1', wallet_uri: WALLET_URI })
    reveal.mockResolvedValueOnce(presented('ceremony-1')).mockResolvedValueOnce(presented('ceremony-1'))

    const wrapper = await openWalletSigning()

    expect(wrapper.text()).not.toContain('Wallet signing failed')
    expect(wrapper.text()).toContain('Signed document accepted and recorded.')
    expect(wrapper.text()).toContain('The signature was recorded, but the contract could not be reloaded')
  })

  it('opens the contract when the provenance chain cannot be read', async () => {
    service.retrieveById.mockResolvedValue({ contract })
    service.getProvenanceChain.mockRejectedValueOnce(new Error('pdf-core unavailable'))

    const wrapper = shallowMount(SecureContractViewerView, {
      global: { stubs: { SigningCeremonyDialog: DialogStub } },
    })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Failed to retrieve the contract')
    expect(wrapper.text()).toContain('Supply agreement')
    expect(wrapper.get('[data-testid="provenance-error"]').text()).toContain('pdf-core unavailable')
  })
})

describe('Signing ceremony dialog refusals and expiry (DCS-FR-SM-23, DCS-IR-SM-04)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    HTMLDialogElement.prototype.showModal = vi.fn()
    HTMLDialogElement.prototype.close = vi.fn()
  })
  afterEach(() => vi.useRealTimers())

  function revealSigning(wrapper: ReturnType<typeof mount<typeof SigningCeremonyDialog>>) {
    return wrapper.vm.reveal({
      contractDid: CONTRACT_DID,
      fieldName: 'did:web:org',
      signingRequest: { ceremonyId: 'ceremony-1', walletUri: WALLET_URI },
    })
  }

  it('a refused wallet signature shows the reason and stops polling', async () => {
    service.getCeremonyStatus.mockResolvedValue({
      ceremony_id: 'ceremony-1',
      status: 'verified',
      refusal_code: 'document_mismatch',
      refusal: 'submitted document does not match the document prepared for signing',
    })
    const wrapper = mount(SigningCeremonyDialog, { attachTo: document.body })
    void revealSigning(wrapper)
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2500)
    expect(document.body.textContent).toContain('submitted document does not match the document prepared for signing')
    expect(document.body.textContent).not.toContain('Waiting for the wallet signature')
    await vi.advanceTimersByTimeAsync(7500)
    expect(service.getCeremonyStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('an expired signing request offers a new request and resolves for republishing', async () => {
    service.getCeremonyStatus.mockResolvedValue({
      ceremony_id: 'ceremony-1',
      status: 'verified',
      request_expires_at: '2020-01-01T00:00:00Z',
    })
    const wrapper = mount(SigningCeremonyDialog, { attachTo: document.body })
    const outcome = revealSigning(wrapper)
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2500)
    expect(document.body.textContent).toContain('The signing request expired.')
    const retry = Array.from(document.body.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === 'Publish a new signing request',
    )
    expect(retry, 'retry button').toBeDefined()
    retry!.click()

    await expect(outcome).resolves.toMatchObject({ isCanceled: false, data: { republish: true } })
    wrapper.unmount()
  })

  it('a refused presentation shows the reason', async () => {
    service.startCeremony.mockResolvedValue({ ceremony_id: 'ceremony-2', wallet_uri: WALLET_URI, status: 'pending' })
    service.getCeremonyStatus.mockResolvedValue({
      ceremony_id: 'ceremony-2',
      status: 'pending',
      refusal_code: 'bad_request',
      refusal: 'the Power of Attorney does not carry the Contract Signer role',
    })
    const wrapper = mount(SigningCeremonyDialog, { attachTo: document.body })
    void wrapper.vm.reveal({ contractDid: CONTRACT_DID, fieldName: 'did:web:org' })
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2500)
    expect(document.body.textContent).toContain('the Power of Attorney does not carry the Contract Signer role')
    wrapper.unmount()
  })
})
