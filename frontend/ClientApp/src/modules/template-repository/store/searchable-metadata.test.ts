import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useDcsDraftStore } from './dcsDraftStore'

/** DCS-FR-TR-10: jurisdiction and industry travel from the loaded template into every save payload. */
describe('template searchable metadata', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('keeps jurisdiction and industry through both document load paths and all payloads', () => {
    const store = useDcsDraftStore()
    const doc = store.templateDocument
    for (const raw of [doc, {}]) {
      store.loadDocument(raw, {
        did: 'did:test:template',
        name: 'Template',
        description: 'Description',
        partyType: 'SINGLE_PARTY',
        jurisdiction: 'DEU',
        industry: 'Energy',
        updated_at: '2026-10-31T00:00:00Z',
      })
      for (const payload of [
        store.templateCreateRequestData,
        store.templateUpdateRequestData,
        store.templateUpdateManageRequestData,
      ]) {
        expect(payload?.jurisdiction).toBe('DEU')
        expect(payload?.industry).toBe('Energy')
      }
    }
    store.loadDocument({}, { did: 'did:test:other', name: '', description: '' })
    expect(store.jurisdiction).toBe('')
  })
})
