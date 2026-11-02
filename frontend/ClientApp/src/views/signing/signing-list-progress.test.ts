import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import SigningListView from './SigningListView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * DCS-FR-SM-24: Contract Managers track the signature progress of a contract
 * per declared field; an APPROVED contract reads as pending only to a signer.
 */

const CONTRACT = {
  did: 'did:web:example.com:contract:1',
  name: 'Supply agreement',
  state: 'APPROVED',
  updated_at: '2026-10-31T10:00:00Z',
}

vi.mock('@/services/signature-management-service', () => ({
  signatureManagementService: {
    retrieveContracts: () => Promise.resolve([CONTRACT]),
    getSignatureView: () =>
      Promise.resolve({ did: CONTRACT.did, contract_state: 'APPROVED', signatures: [], integrity_findings: [] }),
    retrieveById: () =>
      Promise.resolve({
        contract: CONTRACT,
        signature_fields: [
          {
            field_name: 'SignerOne',
            status: 'SIGNED',
            remote: false,
            signer_did: 'did:jwk:one',
            signed_at: '2026-10-31T10:00:00Z',
          },
          { field_name: 'SignerTwo', status: 'UNSIGNED', remote: false },
        ],
      }),
  },
}))

async function mountAs(role: UserRole) {
  setActivePinia(createPinia())
  useAuthStore().user = { holder: 'did:example:holder', issuer: 'did:web:org', roles: [role] }
  const wrapper = shallowMount(SigningListView, { global: { stubs: { RouterLink: true } } })
  await flushPromises()
  return wrapper
}

beforeEach(() => vi.clearAllMocks())

describe('Signing list progress', () => {
  it('shows a manager the per-field signing progress', async () => {
    const wrapper = await mountAs('CONTRACT_MANAGER')
    expect(wrapper.get('[data-testid="signing-status"]').text()).toBe('AWAITING SIGNATURES')

    await wrapper.get('[data-testid="signing-details-toggle"]').trigger('click')
    await flushPromises()

    const progress = wrapper.get('[data-testid="signing-progress"]')
    expect(progress.text()).toContain('1 of 2 signature fields signed')
    const rows = progress.findAll('[data-testid="signing-field"]').map((r) => r.text())
    expect(rows[0]).toContain('SignerOne')
    expect(rows[0]).toContain('did:jwk:one')
    expect(rows[1]).toContain('PENDING')
    expect(rows[1]).toContain('SignerTwo')
  })

  it('shows a signer the contract as pending', async () => {
    const wrapper = await mountAs('CONTRACT_SIGNER')
    expect(wrapper.get('[data-testid="signing-status"]').text()).toBe('PENDING')
  })
})
