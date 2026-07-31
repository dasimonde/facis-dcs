import { createPinia, setActivePinia } from 'pinia'
import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { ONTOLOGY_ASSETS, refreshOntologyDomainFields } from '@template-repository/utils/ontology-domain-fields'
import { compactXsdDatatype } from '@/models/dcs-jsonld'

/**
 * A datatype an imported library declares must reach the emitted
 * dcs:ContractField and survive a save/reload — an xsd:duration field that
 * comes back as xsd:string has lost the only thing that tells a boundary
 * check how to order it, and "PT6H <= PT24H" then compares bytes: "PT6H"
 * sorts AFTER "PT24H", so a six-hour window reads as breaching a 24-hour
 * limit. The degradation is silent, which is what makes it dangerous.
 */

const SHAPES = `
@prefix sh:   <http://www.w3.org/ns/shacl#> .
@prefix xsd:  <http://www.w3.org/2001/XMLSchema#> .
@prefix slah: <https://w3id.org/facis/sla/hosting/v1#> .

slah:SupportCommitmentShape
  a sh:NodeShape ;
  sh:targetClass slah:SupportCommitment ;
  sh:property [
    sh:path slah:responseWindow ;
    sh:name "Response window" ;
    sh:datatype xsd:duration ;
  ] ;
  sh:property [
    sh:path slah:committedFrom ;
    sh:name "Committed from" ;
    sh:datatype xsd:dateTime ;
  ] .
`

const RESPONSE_WINDOW = 'https://w3id.org/facis/sla/hosting/v1#responseWindow'
const COMMITTED_FROM = 'https://w3id.org/facis/sla/hosting/v1#committedFrom'
const COMMITMENT = 'https://w3id.org/facis/sla/hosting/v1#SupportCommitment'

const realFetch = globalThis.fetch

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

beforeAll(async () => {
  globalThis.fetch = (input: RequestInfo | URL) => {
    const route = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url
    if (route === '/api/semantic/schema/list') {
      return Promise.resolve(json([{ name: 'facis-sla-hosting', kind: 'shapes', active_version: 1 }]))
    }
    if (route === '/api/semantic/shapes/facis-sla-hosting') return Promise.resolve(json({ content: SHAPES }))
    return Promise.resolve(new Response(`no stub for ${route}`, { status: 404 }))
  }
  await refreshOntologyDomainFields()
})

afterAll(() => {
  globalThis.fetch = realFetch
})

beforeEach(() => {
  setActivePinia(createPinia())
})

function declareCommitment() {
  const store = useDcsDraftStore()
  store.addClauseWithMeaning({
    title: 'Support',
    content: ['The Provider responds within ', { '@id': 'urn:uuid:field-window' }, '.'],
    fields: [
      {
        id: 'urn:uuid:field-window',
        parameterName: 'responseWindow',
        domainFieldIri: RESPONSE_WINDOW,
        label: 'Response window',
      },
      {
        id: 'urn:uuid:field-from',
        parameterName: 'committedFrom',
        domainFieldIri: COMMITTED_FROM,
        label: 'Committed from',
      },
    ],
    assets: [
      {
        id: 'urn:uuid:asset-commitment',
        classIri: COMMITMENT,
        properties: [
          { fieldId: 'urn:uuid:field-window', path: RESPONSE_WINDOW },
          { fieldId: 'urn:uuid:field-from', path: COMMITTED_FROM },
        ],
      },
    ],
    rule: null,
  })
  return store
}

describe('a declared datatype survives import and round trip', () => {
  it('carries sh:datatype xsd:duration into the hub field definition', () => {
    const asset = ONTOLOGY_ASSETS.find((candidate) => candidate.id === COMMITMENT)
    const window = asset?.properties.find((property) => property.ontologyId === RESPONSE_WINDOW)
    expect(window?.datatype).toBe('xsd:duration')
  })

  it('declares the imported field as xsd:duration, not xsd:string', () => {
    const store = declareCommitment()
    const window = store.contractFields.find((field) => field['@id'] === 'urn:uuid:field-window')
    expect(window?.['dcs:datatype']).toBe('xsd:duration')
  })

  it('keeps xsd:dateTime, which the input-widget vocabulary cannot express either', () => {
    const store = declareCommitment()
    const from = store.contractFields.find((field) => field['@id'] === 'urn:uuid:field-from')
    expect(from?.['dcs:datatype']).toBe('xsd:dateTime')
  })

  // The hard bar: save the template, reload it, read the declaration back.
  it('still reads xsd:duration after the template is saved and reloaded', () => {
    const saved = JSON.parse(JSON.stringify(declareCommitment().templateDocument)) as unknown
    setActivePinia(createPinia())
    const reloaded = useDcsDraftStore()
    reloaded.loadDocument(saved, { did: 'did:web:example:template:sla', name: 'SLA', description: '' })
    const window = reloaded.contractFields.find((field) => field['@id'] === 'urn:uuid:field-window')
    expect(window?.['dcs:datatype']).toBe('xsd:duration')
  })
})

describe('an unrecognised datatype does not silently become a string', () => {
  it.each([
    'http://www.w3.org/2001/XMLSchema#duration',
    'http://www.w3.org/2001/XMLSchema#dateTime',
    'http://www.w3.org/2001/XMLSchema#anyURI',
    'xsd:integer',
  ])('recognises %s', (iri) => {
    expect(compactXsdDatatype(iri)).toBeDefined()
  })

  it.each(['http://www.w3.org/2001/XMLSchema#gYear', 'http://www.w3.org/2001/XMLSchema#time', 'xsd:gMonthDay'])(
    'rejects %s rather than reading it as a string',
    (iri) => {
      expect(() => compactXsdDatatype(iri)).toThrow(/cannot order/)
    },
  )

  it('leaves a non-XSD range undeclared instead of guessing a datatype', () => {
    expect(compactXsdDatatype('https://w3id.org/facis/dcs/ontology/v1#CompanyParty')).toBeUndefined()
    expect(compactXsdDatatype('')).toBeUndefined()
  })
})
