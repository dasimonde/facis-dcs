import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { type ContractTemplate, requirableTemplates } from '@/models/contract-template/contract-template'
import { useDcsDraftStore } from './dcsDraftStore'

/** DCS-FR-TR-02, DCS-FR-TR-23: the level and the required templates travel into every save payload. */
describe('template structure', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('keeps level and required templates through both document load paths and all payloads', () => {
    const store = useDcsDraftStore()
    const doc = store.templateDocument
    for (const raw of [doc, {}]) {
      store.loadDocument(raw, {
        did: 'did:test:sub',
        name: 'Sub',
        description: 'Description',
        partyType: 'SINGLE_PARTY',
        templateLevel: 'SUB_AGREEMENT',
        requiredTemplates: ['did:test:frame'],
        updated_at: '2026-10-31T00:00:00Z',
      })
      for (const payload of [
        store.templateCreateRequestData,
        store.templateUpdateRequestData,
        store.templateUpdateManageRequestData,
      ]) {
        expect(payload?.template_level).toBe('SUB_AGREEMENT')
        expect(payload?.required_templates).toEqual(['did:test:frame'])
      }
    }
    store.loadDocument({}, { did: 'did:test:other', name: '', description: '' })
    expect(store.templateLevel).toBe('AGREEMENT')
    expect(store.requiredTemplates).toEqual([])
  })

  it('offers only approved contract templates of a higher level as requirements', () => {
    const template = (did: string, extra: Partial<ContractTemplate>): ContractTemplate => ({
      did,
      created_by: 'org',
      created_at: '',
      updated_at: '',
      version: 1,
      party_type: 'SINGLE_PARTY',
      template_type: 'CONTRACT_TEMPLATE',
      state: 'REGISTERED',
      ...extra,
    })
    const templates = [
      template('frame', { template_level: 'FRAME_AGREEMENT' }),
      template('frame-draft', { template_level: 'FRAME_AGREEMENT', state: 'DRAFT' }),
      template('component', { template_level: 'FRAME_AGREEMENT', template_type: 'COMPONENT' }),
      template('agreement', { template_level: 'AGREEMENT' }),
      template('legacy', {}),
      template('addendum', { template_level: 'ADDENDUM' }),
    ]
    const dids = (level: Parameters<typeof requirableTemplates>[1], self: string | null = null) =>
      requirableTemplates(templates, level, self).map((t) => t.did)
    expect(dids('SUB_AGREEMENT')).toEqual(['frame'])
    expect(dids('ADDENDUM')).toEqual(['frame', 'agreement', 'legacy'])
    expect(dids('FRAME_AGREEMENT')).toEqual([])
    expect(dids('ADDENDUM', 'agreement')).toEqual(['frame', 'legacy'])
  })
})
