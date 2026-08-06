import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'

/**
 * A clause deleted in the builder must take its declarations with it.
 *
 * assembleCanonicalDocument serializes dcs:blocks whole, so a clause left in
 * the draft without a placement still reaches the contract while nothing
 * renders it. Its prose keeps binding required fields, and both gates read
 * exactly that: the backend refuses the offer
 * (validation.ValidateContractClosed → "prose field binds to unfilled field"),
 * and ContractManagerActions disables the button naming a placeholder the
 * contract no longer shows.
 */

const FIELD = 'urn:uuid:doc#field-monthly-fee'
const OTHER_FIELD = 'urn:uuid:doc#field-term'

function seedClauseWithPlaceholder() {
  const store = useDcsDraftStore()
  const clauseId = store.addClause({
    title: 'Payment',
    content: ['The Customer shall pay ', { '@id': FIELD }, ' per month.'],
  })
  store.contractFields.push({
    '@id': FIELD,
    '@type': 'dcs:ContractField',
    'dcs:label': 'Monthly fee',
    'dcs:required': true,
  })
  const root = store.layout.find((node) => node['dcs:isRoot'])
  if (!root) throw new Error('the seeded draft has no root layout node')
  root['dcs:children'] = { '@list': [{ '@id': clauseId }] }
  return { store, clauseId }
}

/** A second section that places the same clause, the way a clause reused in
 *  two places sits in the layout. */
function placeAlsoUnderSecondSection(store: ReturnType<typeof useDcsDraftStore>, clauseId: string) {
  const sectionId = 'urn:uuid:doc#section-2'
  store.layout.push({ '@id': sectionId, '@type': 'dcs:LayoutNode', 'dcs:children': { '@list': [{ '@id': clauseId }] } })
  const root = store.layout.find((node) => node['dcs:isRoot'])!
  root['dcs:children'] = { '@list': [...root['dcs:children']['@list'], { '@id': sectionId }] }
}

describe('a deleted clause withdraws what it bound', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('drops the clause block, not only its placement, when the builder deletes it', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()

    store.deleteBlock(clauseId)

    expect(store.blocks.some((block) => block['@id'] === clauseId)).toBe(false)
  })

  it('withdraws the field declaration the deleted prose bound', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()

    store.deleteBlock(clauseId)

    expect(store.contractFields.some((field) => field['@id'] === FIELD)).toBe(false)
  })

  it('keeps a clause that is still placed elsewhere, and its field with it', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()
    placeAlsoUnderSecondSection(store, clauseId)

    store.deleteBlock(clauseId)

    expect(store.blocks.some((block) => block['@id'] === clauseId)).toBe(true)
    expect(store.contractFields.some((field) => field['@id'] === FIELD)).toBe(true)
  })

  it('keeps a field another clause still binds', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()
    const survivor = store.addClause({ title: 'Term', content: ['Ends on ', { '@id': FIELD }, '.'] })
    const root = store.layout.find((node) => node['dcs:isRoot'])!
    root['dcs:children'] = { '@list': [{ '@id': clauseId }, { '@id': survivor }] }

    store.deleteBlock(clauseId)

    expect(store.contractFields.some((field) => field['@id'] === FIELD)).toBe(true)
  })

  it('keeps a field a data object still binds', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()
    store.contractData.push({
      '@id': 'urn:uuid:doc#object-1',
      '@type': 'https://w3id.org/facis/sla/hosting/v1#Environment',
      'https://w3id.org/facis/sla/hosting/v1#fee': { '@id': FIELD },
    })

    store.deleteBlock(clauseId)

    expect(store.contractFields.some((field) => field['@id'] === FIELD)).toBe(true)
  })

  it('leaves declarations the deleted clause never bound untouched', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()
    store.contractFields.push({
      '@id': OTHER_FIELD,
      '@type': 'dcs:ContractField',
      'dcs:label': 'Term',
      'dcs:required': true,
    })

    store.deleteBlock(clauseId)

    expect(store.contractFields.some((field) => field['@id'] === OTHER_FIELD)).toBe(true)
  })

  it('drops a rule whose prose the deletion removed', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()
    store.policies.push({
      '@id': 'urn:uuid:doc#policy-1',
      '@type': 'odrl:Duty',
      'dcs:prose': { '@id': clauseId },
      'odrl:action': { '@id': 'dcs:provideCompliantValue' },
    } as never)

    store.deleteBlock(clauseId)

    expect(store.policies).toHaveLength(0)
  })

  it('withdraws the same declarations when the clause list deletes the clause', () => {
    const { store, clauseId } = seedClauseWithPlaceholder()

    store.deleteClause(clauseId)

    expect(store.blocks.some((block) => block['@id'] === clauseId)).toBe(false)
    expect(store.contractFields.some((field) => field['@id'] === FIELD)).toBe(false)
  })
})
