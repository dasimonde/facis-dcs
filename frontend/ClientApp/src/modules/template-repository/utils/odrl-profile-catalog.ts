import { Parser, type Quad } from 'n3'

/**
 * The DCS ODRL profile catalog: the profile IRI, the default constraint
 * action, and the supported constraint operators (with labels and
 * per-parameter-type applicability) driving the obligation editor. Fetched
 * at startup from the Semantic Hub's registered "dcs-odrl-profile" ontology
 * — registering a new version in the hub changes what the editor offers, no
 * rebuild involved — and parsed with N3.
 */

export interface OdrlOperatorDefinition {
  /** Absolute operator IRI. */
  iri: string
  /** Compacted `odrl:`/`dcs:` form — the exact term emitted into documents. */
  term: string
  label: string
  parameterTypes: Set<string>
}

const RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type'
const RDFS_LABEL = 'http://www.w3.org/2000/01/rdf-schema#label'
const ODRL = 'http://www.w3.org/ns/odrl/2/'
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'

async function loadProfileQuads(): Promise<Quad[]> {
  // Raw fetch, deliberately not the app's http client: this module loads at
  // import time (top-level await below), before Pinia exists — and the http
  // client's auth interceptor needs an active Pinia. The hub's resolve
  // routes are public.
  const response = await fetch('/api/semantic/ontology/dcs-odrl-profile', {
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    throw new Error(`Semantic Hub ontology dcs-odrl-profile is unavailable: HTTP ${response.status}`)
  }
  const body = (await response.json()) as { content: string }
  return new Parser().parse(body.content)
}

/** Prefixed form for document emission, matching the hub context's prefixes. */
function compactTerm(iri: string): string {
  if (iri.startsWith(ODRL)) return `odrl:${iri.slice(ODRL.length)}`
  if (iri.startsWith(DCS)) return `dcs:${iri.slice(DCS.length)}`
  return iri
}

function objectValues(quads: Quad[], subject: string, predicateIRI: string): string[] {
  return quads
    .filter((quad) => quad.subject.value === subject && quad.predicate.value === predicateIRI)
    .map((quad) => quad.object.value)
}

function parseProfileIri(quads: Quad[]): string {
  const profile = quads.find((quad) => quad.predicate.value === RDF_TYPE && quad.object.value === `${ODRL}Profile`)
  if (!profile) throw new Error('dcs-odrl-profile declares no odrl:Profile subject.')
  return profile.subject.value
}

function parseDefaultConstraintAction(quads: Quad[], profileIri: string): string {
  const action = objectValues(quads, profileIri, `${DCS}defaultConstraintAction`)[0]
  if (!action) throw new Error('dcs-odrl-profile declares no dcs:defaultConstraintAction.')
  return compactTerm(action)
}

/** Operator individuals in document order — the order the editor offers them in. */
function parseOperators(quads: Quad[]): OdrlOperatorDefinition[] {
  const operators = quads
    .filter((quad) => quad.predicate.value === RDF_TYPE && quad.object.value === `${ODRL}Operator`)
    .map((quad) => quad.subject.value)
    .map((iri) => ({
      iri,
      term: compactTerm(iri),
      label: objectValues(quads, iri, RDFS_LABEL)[0] ?? '',
      parameterTypes: new Set(objectValues(quads, iri, `${DCS}appliesToParameterType`)),
    }))
  const incomplete = operators.find((operator) => !operator.label || !operator.parameterTypes.size)
  if (!operators.length || incomplete) {
    throw new Error(`dcs-odrl-profile operator catalog is incomplete${incomplete ? `: ${incomplete.iri}` : '.'}`)
  }
  return operators
}

const quads = await loadProfileQuads()

/** The DCS ODRL profile IRI declared as `odrl:profile` on every enclosing policy set. */
export const ODRL_PROFILE_IRI: string = parseProfileIri(quads)

/** The action IRI dcsDraftStore attaches to every generated field-constraint ODRL rule. */
export const DEFAULT_FIELD_CONSTRAINT_ACTION: string = parseDefaultConstraintAction(quads, ODRL_PROFILE_IRI)

/** The profile's supported constraint operators, in the hub-declared order. */
export const ODRL_OPERATORS: readonly OdrlOperatorDefinition[] = parseOperators(quads)
