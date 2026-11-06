import { describe, expect, it } from 'vitest'
import { activeFilters, archiveResultsToCsv, deleteSavedQuery, loadSavedQueries, saveQuery } from './archive-search'
import type { ArchivedContract } from '@/services/archive-service'

function memoryStorage(): Storage {
  const items = new Map<string, string>()
  return {
    get length() {
      return items.size
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => Array.from(items.keys())[index] ?? null,
    removeItem: (key) => void items.delete(key),
    setItem: (key, value) => void items.set(key, value),
  }
}

describe('archive search (DCS-FR-CSA-22)', () => {
  it('keeps only the filters that are set', () => {
    expect(activeFilters({ state: 'SIGNED', name: ' ', party: '', contract_version: 2 })).toEqual({
      state: 'SIGNED',
      contract_version: 2,
    })
  })

  it('saves, replaces and deletes named queries', () => {
    const storage = memoryStorage()
    saveQuery('signed', { state: 'SIGNED', name: '' }, storage)
    saveQuery('signed', { state: 'TERMINATED' }, storage)
    saveQuery('tagged', { tag: 'audit-2026' }, storage)
    expect(loadSavedQueries(storage)).toEqual([
      { name: 'signed', query: { state: 'TERMINATED' } },
      { name: 'tagged', query: { tag: 'audit-2026' } },
    ])
    expect(deleteSavedQuery('signed', storage)).toEqual([{ name: 'tagged', query: { tag: 'audit-2026' } }])
  })

  it('reads an unusable store as no saved queries', () => {
    const storage = memoryStorage()
    storage.setItem('dcs.archive.saved-queries', '{not json')
    expect(loadSavedQueries(storage)).toEqual([])
  })

  it('exports the results as CSV with quoted cells', () => {
    const result = {
      did: 'did:web:a',
      contract_version: 2,
      state: 'SIGNED',
      name: 'Supply, "Q3"',
      created_by: 'did:web:org',
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-02T00:00:00Z',
      archive_tags: ['audit', '2026'],
    } as ArchivedContract
    const lines = archiveResultsToCsv([result]).split('\r\n')
    expect(lines[0]).toBe(
      'did,contract_version,state,name,description,created_by,created_at,updated_at,exp_date,archive_summary,archive_tags',
    )
    expect(lines[1]).toBe(
      'did:web:a,2,SIGNED,"Supply, ""Q3""",,did:web:org,2026-10-01T00:00:00Z,2026-10-02T00:00:00Z,,,audit; 2026',
    )
  })
})
