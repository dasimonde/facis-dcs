import { createPinia, setActivePinia } from 'pinia'
import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { ONTOLOGY_ASSETS, refreshOntologyDomainFields } from '@template-repository/utils/ontology-domain-fields'

/**
 * A field declared by importing a hub shapes graph must carry what the
 * property shape declares — its sh:datatype and its value constraints — into
 * the emitted dcs:ContractField. Otherwise every imported domain library
 * yields untyped, unconstrained fields and a filled value is a string literal
 * in a typed slot, which the same shapes graph then refuses.
 */

const SHAPES = `
@prefix sh:   <http://www.w3.org/ns/shacl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix xsd:  <http://www.w3.org/2001/XMLSchema#> .
@prefix slah: <https://w3id.org/facis/sla/hosting/v1#> .

slah:ManagedKubernetesEnvironmentShape
  a sh:NodeShape ;
  sh:targetClass slah:ManagedKubernetesEnvironment ;
  rdfs:label "Managed Kubernetes Environment" ;
  sh:property [
    sh:path slah:provisionedNodes ;
    sh:name "Provisioned nodes" ;
    sh:datatype xsd:integer ;
    sh:minInclusive 1 ;
    sh:maxInclusive 500 ;
  ] ;
  sh:property [
    sh:path slah:nodeClass ;
    sh:name "Node class" ;
    sh:datatype xsd:string ;
    sh:in ( "GENERAL_PURPOSE" "MEMORY_OPTIMIZED" ) ;
  ] .
`

const NODES = 'https://w3id.org/facis/sla/hosting/v1#provisionedNodes'
const NODE_CLASS = 'https://w3id.org/facis/sla/hosting/v1#nodeClass'
const ENVIRONMENT = 'https://w3id.org/facis/sla/hosting/v1#ManagedKubernetesEnvironment'

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

/** Declares the imported asset in a clause, exactly as the clause editor does. */
function declareAsset() {
  const store = useDcsDraftStore()
  const assetLocalId = 'urn:uuid:asset-1'
  store.addClauseWithMeaning({
    title: 'Environment',
    content: ['The Provider operates ', { '@id': 'urn:uuid:field-nodes' }, ' nodes.'],
    fields: [
      { id: 'urn:uuid:field-nodes', parameterName: 'provisionedNodes', domainFieldIri: NODES, label: 'Nodes' },
      { id: 'urn:uuid:field-class', parameterName: 'nodeClass', domainFieldIri: NODE_CLASS, label: 'Node class' },
    ],
    assets: [
      {
        id: assetLocalId,
        classIri: ENVIRONMENT,
        properties: [
          { fieldId: 'urn:uuid:field-nodes', path: NODES },
          { fieldId: 'urn:uuid:field-class', path: NODE_CLASS },
        ],
      },
    ],
    rule: null,
  })
  return store
}

describe('contract fields derived from an imported shapes graph', () => {
  it('offers the imported asset with its property shapes', () => {
    const asset = ONTOLOGY_ASSETS.find((candidate) => candidate.id === ENVIRONMENT)
    expect(asset?.properties.map((property) => property.ontologyId)).toEqual([NODES, NODE_CLASS])
  })

  it('carries sh:datatype and sh:minInclusive/sh:maxInclusive into the declared field', () => {
    const store = declareAsset()
    const nodes = store.contractFields.find((field) => field['@id'] === 'urn:uuid:field-nodes')
    expect(nodes?.['dcs:datatype']).toBe('xsd:integer')
    expect(nodes?.['dcs:valueConstraint']?.min).toBe(1)
    expect(nodes?.['dcs:valueConstraint']?.max).toBe(500)
    expect(nodes?.['dcs:shape']).toEqual({ '@id': NODES })
  })

  it('carries sh:in into the declared field as its allowed values', () => {
    const store = declareAsset()
    const nodeClass = store.contractFields.find((field) => field['@id'] === 'urn:uuid:field-class')
    expect(nodeClass?.['dcs:datatype']).toBe('xsd:string')
    expect(nodeClass?.['dcs:valueConstraint']?.allowedValues).toEqual(['GENERAL_PURPOSE', 'MEMORY_OPTIMIZED'])
  })
})
