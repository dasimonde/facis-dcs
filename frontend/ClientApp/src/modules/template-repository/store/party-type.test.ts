import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { partyTypes } from '@/types/party-type'
import { useDcsDraftStore } from './dcsDraftStore'

describe('explicit party classification', () => {
  beforeEach(() => setActivePinia(createPinia()))
  it('requires an explicit selection and clears it on reset', () => {
    const store = useDcsDraftStore()
    expect(store.partyType).toBeUndefined()
    expect(() => store.templateCreateRequestData).toThrow('Party type is required')
    store.partyType = 'MULTI_PARTY'
    expect(store.templateCreateRequestData.party_type).toBe('MULTI_PARTY')
    store.reset()
    expect(store.partyType).toBeUndefined()
  })
  it.each(partyTypes)('preserves $value through both document load paths and all payloads', ({ value }) => {
    const store = useDcsDraftStore()
    const doc = store.templateDocument
    for (const raw of [doc, {}]) {
      store.loadDocument(raw, {
        did: 'did:test:template',
        name: 'Template',
        description: 'Description',
        partyType: value,
        updated_at: '2026-10-16T00:00:00Z',
      })
      expect(store.partyType).toBe(value)
      expect(store.templateCreateRequestData.party_type).toBe(value)
      expect(store.templateUpdateRequestData?.party_type).toBe(value)
      expect(store.templateUpdateManageRequestData?.party_type).toBe(value)
      expect(store.templateDocument).not.toHaveProperty('party_type')
    }
    store.loadDocument({}, { did: 'did:test:other', name: '', description: '' })
    expect(store.partyType).toBeUndefined()
  })
})
