import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import ComplianceViewerView from './ComplianceViewerView.vue'

/**
 * DCS-IR-SM-08: the Compliance Viewer filters contracts by their compliance
 * result, shows signer, credential and outcome per audit entry, and alerts
 * when the trust registry or a status list could not be reached.
 */

const FLAGGED = { did: 'did:web:example.com:contract:1', name: 'Flagged agreement', state: 'SIGNED' }
const UNCHECKED = { did: 'did:web:example.com:contract:2', name: 'Other agreement', state: 'SIGNED' }

vi.mock('@/services/signature-management-service', () => ({
  signatureManagementService: {
    retrieveContracts: () => Promise.resolve([FLAGGED, UNCHECKED]),
    getSignatureView: () =>
      Promise.resolve({
        did: FLAGGED.did,
        contract_state: 'SIGNED',
        signatures: [
          {
            signer_did: 'did:example:signer',
            credential_type: 'AES',
            status: 'SIGNED',
            format: 'PAdES',
            credential_status: 'UNKNOWN',
            credential_status_reason: 'status list https://issuer.example/status is unreachable',
          },
        ],
        integrity_findings: ['Integrity check failed: re-render mismatch'],
      }),
    getAudit: () =>
      Promise.resolve([
        {
          id: 7,
          component: 'SIGNATURE_MANAGEMENT',
          event_type: 'SIGN_SIGNATURE',
          created_at: '2026-10-31T10:00:00Z',
          event_data: { signer_did: 'did:jwk:signer', credential_type: 'AES', outcome: 'FAILED', error: 'nonce' },
        },
      ]),
  },
}))

const ModalStub = defineComponent({
  name: 'ConfirmationModal',
  setup(_, { expose }) {
    expose({ reveal: () => Promise.resolve({ isCanceled: false, data: 'quarterly audit' }) })
    return () => null
  },
})

async function mountAndSelect() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { holder: 'did:example:holder', issuer: 'did:web:org', roles: ['AUDITOR'] }
  const wrapper = shallowMount(ComplianceViewerView, {
    global: { plugins: [pinia], stubs: { ConfirmationModal: ModalStub } },
  })
  await flushPromises()
  await wrapper.get('[data-testid="compliance-contract-row"]').trigger('click')
  await flushPromises()
  return wrapper
}

function rowNames(wrapper: Awaited<ReturnType<typeof mountAndSelect>>) {
  return wrapper.findAll('[data-testid="compliance-contract-row"]').map((r) => r.text())
}

describe('Compliance Viewer compliance status, audit facts and registry alert', () => {
  it('filters contracts by the result of their last check', async () => {
    const wrapper = await mountAndSelect()
    const filter = wrapper.get('[data-testid="compliance-status-filter"]')

    await filter.setValue('fail')
    expect(rowNames(wrapper)).toEqual([expect.stringContaining('Flagged agreement')])
    await filter.setValue('unchecked')
    expect(rowNames(wrapper)).toEqual([expect.stringContaining('Other agreement')])
  })

  it('alerts when a credential status list could not be reached', async () => {
    const wrapper = await mountAndSelect()
    expect(wrapper.get('[data-testid="registry-unreachable"]').text()).toContain(
      'status list https://issuer.example/status is unreachable',
    )
  })

  it('shows signer, credential and outcome of each audit entry', async () => {
    const wrapper = await mountAndSelect()
    await wrapper
      .findAll('[role="tab"]')
      .find((tab) => tab.text() === 'Audit Reports')!
      .trigger('click')
    await wrapper
      .findAll('button')
      .find((b) => b.text().trim() === 'Load Audit Report')!
      .trigger('click')
    await flushPromises()

    const cells = wrapper
      .get('[data-testid="audit-entry"]')
      .findAll('td')
      .map((td) => td.text())
    expect(cells).toEqual(expect.arrayContaining(['did:jwk:signer', 'AES', 'FAILED']))
  })
})
