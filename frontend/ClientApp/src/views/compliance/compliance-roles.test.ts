import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import ComplianceViewerView from './ComplianceViewerView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-IR-SM-04/-05/-06/-07: the compliance users of the Signature Compliance
 * Viewer validate the applied signatures, revoke a signature and run the
 * compliance check.
 */

const CONTRACT = { did: 'did:web:example.com:contract:1', name: 'Supply agreement', state: 'SIGNED' }

const { validateSignature, complianceCheck } = vi.hoisted(() => ({
  validateSignature: vi.fn(() => Promise.resolve({ findings: [] })),
  complianceCheck: vi.fn(() => Promise.resolve({ findings: [] })),
}))

vi.mock('@/services/signature-management-service', () => ({
  signatureManagementService: {
    retrieveContracts: () => Promise.resolve([CONTRACT]),
    getSignatureView: () =>
      Promise.resolve({
        did: CONTRACT.did,
        contract_state: 'SIGNED',
        signatures: [{ signer_did: 'did:example:signer', credential_type: 'AES', status: 'SIGNED', format: 'PAdES' }],
        integrity_findings: [],
      }),
    validateSignature,
    complianceCheck,
  },
}))

async function mountAs(role: UserRole) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { holder: 'did:example:holder', issuer: 'did:web:org', roles: [role] }
  const wrapper = shallowMount(ComplianceViewerView, { global: { plugins: [pinia] } })
  await flushPromises()
  await wrapper.get('[data-testid="compliance-contract-row"]').trigger('click')
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof mountAs>>, label: string) {
  const found = wrapper.findAll('button').find((b) => b.text().trim() === label)
  expect(found, `button ${label}`).toBeDefined()
  return found!
}

async function openTab(wrapper: Awaited<ReturnType<typeof mountAs>>, label: string) {
  await wrapper
    .findAll('[role="tab"]')
    .find((tab) => tab.text() === label)!
    .trigger('click')
}

describe('Signature Compliance Viewer for compliance users', () => {
  beforeEach(() => vi.clearAllMocks())

  it.each<UserRole>(['AUDITOR', 'COMPLIANCE_OFFICER'])('lets %s validate the signatures', async (role) => {
    const wrapper = await mountAs(role)
    const validate = button(wrapper, 'Validate')
    expect(validate.attributes('disabled')).toBeUndefined()
    await validate.trigger('click')
    await flushPromises()
    expect(validateSignature).toHaveBeenCalledWith(CONTRACT.did)
  })

  it.each<UserRole>(['AUDITOR', 'COMPLIANCE_OFFICER'])('lets %s run compliance and revoke', async (role) => {
    const wrapper = await mountAs(role)
    await openTab(wrapper, 'Compliance Checks')
    await button(wrapper, 'Run Compliance').trigger('click')
    await flushPromises()
    expect(complianceCheck).toHaveBeenCalledWith(CONTRACT.did)

    await openTab(wrapper, 'Revocation')
    expect(button(wrapper, 'Revoke').attributes('disabled')).toBeUndefined()
  })

  it('keeps the actions closed to a role outside the viewer', async () => {
    const wrapper = await mountAs('CONTRACT_OBSERVER')
    expect(button(wrapper, 'Validate').attributes('disabled')).toBeDefined()
  })
})
