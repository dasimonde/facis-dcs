import { Parser, type Quad } from 'n3'
import { shallowReactive } from 'vue'
import { compactXsdDatatype, type XsdDatatype } from '@/models/dcs-jsonld'
import type {
  DomainFieldDefinition,
  SemanticParameterType,
  SemanticValueConstraint,
  SemanticValueOption,
} from '@template-repository/models/contract-template'

/**
 * The builder's pickable domain-field vocabulary, discovered at startup from
 * every schema the Semantic Hub holds — each registered ontology's
 * dcs:DomainField individuals and each registered shapes graph's property
 * shapes — and parsed with N3. Registering a schema in the hub (a new ontology
 * version, an imported Gaia-X profile) changes what the builder offers with no
 * rebuild and no hardcoded schema name.
 */

const RDF = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#'
const RDF_TYPE = `${RDF}type`
const RDF_NIL = `${RDF}nil`
const RDFS = 'http://www.w3.org/2000/01/rdf-schema#'
const OWL_CLASS = 'http://www.w3.org/2002/07/owl#Class'
const SKOS = 'http://www.w3.org/2004/02/skos/core#'
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'
const SH = 'http://www.w3.org/ns/shacl#'
const CONTRACT_PARTY_ROLE_CODE = `${DCS}ContractPartyRoleCode`
const TECHNICAL_SHAPE = `${DCS}TechnicalShape`

export class OntologyGraph {
  private bySubject = new Map<string, Quad[]>()
  private readonly quads: Quad[]

  constructor(quads: Quad[]) {
    this.quads = quads
    for (const quad of quads) {
      const key = quad.subject.value
      const list = this.bySubject.get(key)
      if (list) list.push(quad)
      else this.bySubject.set(key, [quad])
    }
  }

  subjectsOfType(typeIRI: string): string[] {
    const subjects: string[] = []
    for (const [subject, quads] of this.bySubject) {
      if (quads.some((quad) => quad.predicate.value === RDF_TYPE && quad.object.value === typeIRI)) {
        subjects.push(subject)
      }
    }
    return subjects
  }

  values(subject: string, predicateIRI: string): string[] {
    return (this.bySubject.get(subject) ?? [])
      .filter((quad) => quad.predicate.value === predicateIRI)
      .map((quad) => quad.object.value)
  }

  first(subject: string, predicateIRI: string): string {
    return this.values(subject, predicateIRI)[0] ?? ''
  }

  firstNumber(subject: string, predicateIRI: string): number | undefined {
    const raw = this.first(subject, predicateIRI)
    if (raw === '') return undefined
    const parsed = Number(raw)
    return Number.isNaN(parsed) ? undefined : parsed
  }

  subjects(): string[] {
    return [...this.bySubject.keys()]
  }

  allQuads(): readonly Quad[] {
    return this.quads
  }
}

interface SchemaListEntry {
  name: string
  kind: string
  active_version?: number
  latest_version?: number
  updated_at?: string
}

// Raw fetch, deliberately not the app's http client: this module loads at
// import time (top-level await below), before Pinia exists — and the http
// client's auth interceptor needs an active Pinia. The hub's resolve and
// list routes are public.
export async function fetchHubJson<T>(route: string): Promise<T> {
  const response = await fetch(route, { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`Semantic Hub route ${route} is unavailable: HTTP ${response.status}`)
  }
  return (await response.json()) as T
}

/** The hub route serving a registered schema's content, by kind. */
function schemaContentRoute(kind: string, name: string): string | null {
  const encoded = encodeURIComponent(name)
  switch (kind) {
    case 'ontology':
      return `/api/semantic/ontology/${encoded}`
    case 'shapes':
      return `/api/semantic/shapes/${encoded}`
    default:
      return null
  }
}

async function loadSchemaGraph(kind: string, name: string): Promise<OntologyGraph> {
  const route = schemaContentRoute(kind, name)
  if (!route) throw new Error(`No content route for schema kind ${kind}`)
  const body = await fetchHubJson<{ content: string }>(route)
  return new OntologyGraph(new Parser().parse(body.content))
}

export function localName(iri: string): string {
  return iri.replace(/^.*[:#/]/, '')
}

export function formatOntologyLabel(value: string): string {
  const spaced = value.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[-_]+/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function parseValueOptions(graph: OntologyGraph): ReadonlyMap<string, SemanticValueOption> {
  const options = new Map<string, SemanticValueOption>()
  for (const subject of graph.subjectsOfType(`${SKOS}Concept`)) {
    const value = graph.first(subject, `${SKOS}notation`)
    if (!value) continue
    const catalogIri = graph.first(subject, `${SKOS}inScheme`)
    const catalog = catalogIri
      ? {
          iri: catalogIri,
          label:
            graph.first(catalogIri, `${SKOS}prefLabel`) ||
            graph.first(catalogIri, `${RDFS}label`) ||
            formatOntologyLabel(localName(catalogIri)),
        }
      : undefined
    options.set(subject, {
      value,
      label: graph.first(subject, `${SKOS}prefLabel`) || undefined,
      symbol: graph.first(subject, `${DCS}valueSymbol`) || undefined,
      iri: subject,
      catalog,
    })
  }
  return options
}

function parseValueConstraints(graph: OntologyGraph): ReadonlyMap<string, SemanticValueConstraint> {
  const valueOptions = parseValueOptions(graph)
  const constraints = new Map<string, SemanticValueConstraint>()
  for (const subject of graph.subjectsOfType(`${DCS}ValueConstraint`)) {
    const declaredAllowedValues = graph.values(subject, `${DCS}allowedValue`)
    const valueCatalogIri = graph.first(subject, `${DCS}valueCatalog`)
    const valueCatalog = valueCatalogIri
      ? {
          iri: valueCatalogIri,
          label:
            graph.first(valueCatalogIri, `${SKOS}prefLabel`) ||
            graph.first(valueCatalogIri, `${RDFS}label`) ||
            formatOntologyLabel(localName(valueCatalogIri)),
        }
      : undefined
    const catalogOptions = [...valueOptions.values()].filter((option) => option.catalog?.iri === valueCatalogIri)
    const selectedOptions = declaredAllowedValues.length
      ? catalogOptions.filter((option) =>
          declaredAllowedValues.some((allowed) => allowed === option.value || allowed === option.iri),
        )
      : catalogOptions
    constraints.set(subject, {
      iri: subject,
      format: (graph.first(subject, `${DCS}format`) || undefined) as SemanticValueConstraint['format'],
      pattern: graph.first(subject, `${DCS}pattern`) || undefined,
      allowedValues: declaredAllowedValues.length
        ? declaredAllowedValues
        : selectedOptions.map((option) => option.value),
      valueOptions: selectedOptions,
      valueCatalog,
      allowedValuesRef: graph.first(subject, `${DCS}allowedValuesRef`) || undefined,
      odrlLeftOperands: graph.values(subject, `${DCS}odrlLeftOperand`),
      min: graph.firstNumber(subject, `${DCS}minInclusive`),
      max: graph.firstNumber(subject, `${DCS}maxInclusive`),
      description: graph.first(subject, `${RDFS}label`) || undefined,
    })
  }
  return constraints
}

export interface ContractPartyRoleOption {
  value: string
  label: string
}

function parseContractPartyRoles(
  graph: OntologyGraph,
  constraints = parseValueConstraints(graph),
): ContractPartyRoleOption[] {
  const roles = new Map<string, ContractPartyRoleOption>()
  for (const subject of graph.subjects()) {
    if (graph.first(subject, `${RDFS}range`) !== CONTRACT_PARTY_ROLE_CODE) continue
    const constraint = constraints.get(graph.first(subject, `${DCS}hasValueConstraint`))
    for (const option of constraint?.valueOptions ?? []) {
      if (!option.iri || !option.label) continue
      roles.set(option.iri, { value: option.iri, label: option.label })
    }
  }
  return [...roles.values()]
}

function parseClassLabels(graph: OntologyGraph): ReadonlyMap<string, string> {
  const labels = new Map<string, string>()
  for (const subject of [...graph.subjectsOfType(`${RDFS}Class`), ...graph.subjectsOfType(OWL_CLASS)]) {
    labels.set(subject, graph.first(subject, `${RDFS}label`) || localName(subject))
  }
  return labels
}

/** Maps a field's declared datatype to the builder's parameter type — which
 *  input widget the field gets. The widget vocabulary has no duration and no
 *  instant, so it is NOT the field's datatype: that is carried separately (see
 *  DomainFieldDefinition.datatype) and must never be re-derived from here. */
function parameterTypeForDatatype(datatype?: XsdDatatype): SemanticParameterType {
  switch (datatype) {
    case 'xsd:decimal':
      return 'decimal'
    case 'xsd:integer':
      return 'integer'
    case 'xsd:boolean':
      return 'boolean'
    case 'xsd:date':
      return 'date'
    default:
      return 'string'
  }
}

/** The dcs:parameterName a field's IRI encodes: its local name without the "field-" marker, dot-separated. */
function parameterNameFor(fieldIRI: string): string {
  return localName(fieldIRI)
    .replace(/^field-/, '')
    .replace(/-/g, '.')
}

function cloneConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
    valueCatalog: constraint.valueCatalog ? { ...constraint.valueCatalog } : undefined,
    odrlLeftOperands: constraint.odrlLeftOperands ? [...constraint.odrlLeftOperands] : undefined,
  }
}

function parseOntologyDomainFields(
  graph: OntologyGraph,
  constraints = parseValueConstraints(graph),
): DomainFieldDefinition[] {
  const classLabels = parseClassLabels(graph)

  return graph
    .subjectsOfType(`${DCS}DomainField`)
    .map((subject) => {
      const range = graph.first(subject, `${RDFS}range`)
      const label = graph.first(subject, `${RDFS}label`)
      if (!range || !label) {
        throw new Error(`Ontology domain field ${subject} is incomplete.`)
      }
      const valueConstraintRef = graph.first(subject, `${DCS}hasValueConstraint`)
      const valueConstraint = valueConstraintRef ? cloneConstraint(constraints.get(valueConstraintRef)) : undefined
      const domain = graph.first(subject, `${RDFS}domain`) || undefined
      // An rdfs:range naming a class (an object-valued field) yields no
      // datatype; an XSD range DCS cannot order throws out of here rather
      // than degrading to xsd:string.
      const datatype = compactXsdDatatype(range)
      // A field is an enum when its constraint enumerates allowed values.
      const type: SemanticParameterType = valueConstraint?.allowedValues?.length
        ? 'enum'
        : parameterTypeForDatatype(datatype)
      return {
        ontologyId: subject,
        parameterName: parameterNameFor(subject),
        type,
        datatype,
        label,
        domain,
        domainLabel: domain ? classLabels.get(domain) : undefined,
        valueConstraint,
      }
    })
    .sort((left, right) => left.ontologyId.localeCompare(right.ontologyId))
}

/** Reads an RDF collection (rdf:first/rdf:rest) into its member IRIs/literals. */
export function readRdfList(graph: OntologyGraph, head: string): string[] {
  const members: string[] = []
  let node = head
  const guard = new Set<string>()
  while (node && node !== RDF_NIL && !guard.has(node)) {
    guard.add(node)
    const first = graph.first(node, `${RDF}first`)
    if (first) members.push(first)
    node = graph.first(node, `${RDF}rest`)
  }
  return members
}

/**
 * A hub asset: a SHACL NodeShape's target class (an imported Gaia-X
 * ServiceOffering/DataResource, a registered entity type) plus the fields its
 * property shapes describe. Declaring an asset makes it an ODRL target; its
 * properties become the fields constrained or filled for that asset.
 */
export interface HubAsset {
  /** The sh:targetClass IRI — the asset type's identity. */
  id: string
  label: string
  properties: DomainFieldDefinition[]
  source?: { name: string; kind: string }
}

/** Builds a domain field from a SHACL property shape (a node carrying sh:path). */
function buildPropertyField(graph: OntologyGraph, propShape: string, path: string): DomainFieldDefinition {
  const allowedValues = readRdfList(graph, graph.first(propShape, `${SH}in`)).filter(Boolean)
  const datatype = graph.first(propShape, `${SH}datatype`)
  const label =
    graph.first(propShape, `${SH}name`) ||
    graph.first(propShape, `${RDFS}label`) ||
    formatOntologyLabel(localName(path))
  const pattern = graph.first(propShape, `${SH}pattern`) || undefined
  const min = graph.firstNumber(propShape, `${SH}minInclusive`)
  const max = graph.firstNumber(propShape, `${SH}maxInclusive`)
  const hasConstraint = allowedValues.length > 0 || pattern !== undefined || min !== undefined || max !== undefined
  // A property with no sh:datatype and no enum is object-valued (sh:class /
  // sh:node) — filled with a reference/identifier, carried as a string.
  const compactDatatype = compactXsdDatatype(datatype)
  const type: SemanticParameterType = allowedValues.length ? 'enum' : parameterTypeForDatatype(compactDatatype)
  return {
    ontologyId: path,
    parameterName: parameterNameFor(path),
    type,
    datatype: compactDatatype,
    label,
    valueConstraint: hasConstraint
      ? {
          pattern,
          min,
          max,
          allowedValues: allowedValues.length ? allowedValues : undefined,
          valueOptions: allowedValues.map((value) => ({ value })),
        }
      : undefined,
  }
}

/**
 * Extracts assets from a SHACL shapes graph: every non-technical NodeShape
 * with a sh:targetClass becomes a pickable asset, its property shapes its
 * fields. Shape authors opt out of the picker by typing an envelope or
 * validation-only shape as dcs:TechnicalShape.
 */
function parseShapesAssets(graph: OntologyGraph): HubAsset[] {
  const assets: HubAsset[] = []
  const seenClass = new Set<string>()
  for (const nodeShape of graph.subjectsOfType(`${SH}NodeShape`)) {
    const targetClass = graph.first(nodeShape, `${SH}targetClass`)
    const isTechnicalShape = graph.values(nodeShape, RDF_TYPE).includes(TECHNICAL_SHAPE)
    if (!targetClass || isTechnicalShape || seenClass.has(targetClass)) continue
    seenClass.add(targetClass)
    const label =
      graph.first(nodeShape, `${RDFS}label`) ||
      graph.first(targetClass, `${RDFS}label`) ||
      formatOntologyLabel(localName(targetClass))
    const properties: DomainFieldDefinition[] = []
    const seenPath = new Set<string>()
    for (const propShape of graph.values(nodeShape, `${SH}property`)) {
      const path = graph.first(propShape, `${SH}path`)
      if (!path || seenPath.has(path)) continue
      seenPath.add(path)
      properties.push(buildPropertyField(graph, propShape, path))
    }
    assets.push({ id: targetClass, label, properties })
  }
  return assets
}

/**
 * The builder's pickable vocabulary, discovered from the whole Semantic Hub:
 * each registered ontology contributes its dcs:DomainField individuals (flat
 * data fields); each registered shapes graph contributes its non-technical
 * targeted NodeShapes as assets. Registering a schema in the hub — including
 * an imported Gaia-X profile — makes its objects pickable, with no hardcoded
 * schema name.
 */
async function loadHub(): Promise<{
  fields: DomainFieldDefinition[]
  assets: HubAsset[]
  constraints: SemanticValueConstraint[]
  partyRoleOptions: ContractPartyRoleOption[]
}> {
  const inventory = await fetchHubJson<SchemaListEntry[]>('/api/semantic/schema/list')
  hubFingerprint = fingerprintOf(inventory)
  const loadedSources = await Promise.all(
    inventory
      .filter((entry) => entry.kind === 'ontology' || entry.kind === 'shapes')
      .map(async (entry) => {
        const source = { name: entry.name, kind: entry.kind }
        try {
          const graph = await loadSchemaGraph(entry.kind, entry.name)
          return { source, graph }
        } catch {
          // A single malformed schema must not blank the whole picker.
          return null
        }
      }),
  )
  const sources = loadedSources.filter((entry): entry is NonNullable<typeof entry> => entry !== null)
  const ontologySources = sources.filter((entry) => entry.source.kind === 'ontology')
  const ontologyGraph = new OntologyGraph(ontologySources.flatMap((entry) => [...entry.graph.allQuads()]))
  const constraints = parseValueConstraints(ontologyGraph)
  const perSource = sources.map(({ source, graph }) => {
    if (source.kind === 'ontology') {
      return {
        fields: parseOntologyDomainFields(graph, constraints).map((field) => ({ ...field, source })),
        assets: [] as HubAsset[],
      }
    }
    return {
      fields: [] as DomainFieldDefinition[],
      assets: parseShapesAssets(graph).map((asset) => ({ ...asset, source })),
    }
  })

  const bySource = (a?: { name: string }, b?: { name: string }) => (a?.name ?? '').localeCompare(b?.name ?? '')
  const fieldsById = new Map<string, DomainFieldDefinition>()
  for (const field of perSource.flatMap((p) => p.fields)) {
    if (!fieldsById.has(field.ontologyId)) fieldsById.set(field.ontologyId, field)
  }
  const assetsById = new Map<string, HubAsset>()
  for (const asset of perSource.flatMap((p) => p.assets)) {
    if (!assetsById.has(asset.id)) assetsById.set(asset.id, asset)
  }
  return {
    fields: [...fieldsById.values()].sort((l, r) => bySource(l.source, r.source) || l.label.localeCompare(r.label)),
    assets: [...assetsById.values()].sort((l, r) => bySource(l.source, r.source) || l.label.localeCompare(r.label)),
    constraints: [...constraints.values()],
    partyRoleOptions: parseContractPartyRoles(ontologyGraph, constraints),
  }
}

let hubFingerprint = ''

function fingerprintOf(inventory: SchemaListEntry[]): string {
  return inventory
    .map((e) => `${e.name}|${e.kind}|${e.active_version ?? ''}|${e.latest_version ?? ''}|${e.updated_at ?? ''}`)
    .sort()
    .join(';')
}

type HubVocabulary = Awaited<ReturnType<typeof loadHub>>

// Registered libraries can be megabytes of Turtle (the Gaia-X development
// shapes are ~2.4 MB), and this module re-parses them on every full page
// load — so the parsed vocabulary rides sessionStorage, keyed by the hub
// inventory fingerprint. A quota failure just means parsing again next load.
const HUB_CACHE_KEY = 'dcs.hub.vocabulary.v4'

function readHubCache(fingerprint: string): HubVocabulary | null {
  try {
    const raw = sessionStorage.getItem(HUB_CACHE_KEY)
    if (!raw) return null
    const cached = JSON.parse(raw) as { fingerprint: string; hub: HubVocabulary }
    return cached.fingerprint === fingerprint ? cached.hub : null
  } catch {
    return null
  }
}

function writeHubCache(fingerprint: string, vocabulary: HubVocabulary): void {
  try {
    sessionStorage.setItem(HUB_CACHE_KEY, JSON.stringify({ fingerprint, hub: vocabulary }))
  } catch {
    // Storage quota or unavailable storage — the in-memory copy still serves
    // this page view.
  }
}

async function loadHubCached(): Promise<HubVocabulary> {
  const inventory = await fetchHubJson<SchemaListEntry[]>('/api/semantic/schema/list')
  const fingerprint = fingerprintOf(inventory)
  const cached = readHubCache(fingerprint)
  if (cached) {
    hubFingerprint = fingerprint
    return cached
  }
  const fresh = await loadHub()
  writeHubCache(hubFingerprint, fresh)
  return fresh
}

const hub = await loadHubCached()
const reactiveFields = shallowReactive<DomainFieldDefinition[]>(hub.fields)
const reactiveAssets = shallowReactive<HubAsset[]>(hub.assets)
const reactiveConstraints = shallowReactive<SemanticValueConstraint[]>(hub.constraints)
const reactivePartyRoleOptions = shallowReactive<ContractPartyRoleOption[]>(hub.partyRoleOptions)

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = reactiveFields
export const ONTOLOGY_ASSETS: readonly HubAsset[] = reactiveAssets
export const ONTOLOGY_VALUE_CONSTRAINTS: readonly SemanticValueConstraint[] = reactiveConstraints
export const CONTRACT_PARTY_ROLE_OPTIONS: readonly ContractPartyRoleOption[] = reactivePartyRoleOptions

let refreshInFlight: Promise<void> | null = null

/**
 * Re-reads the Semantic Hub and updates the exported pickable vocabulary in
 * place — a schema registered after app startup becomes pickable without a
 * page reload. Cheap when nothing changed: the schema inventory is
 * fingerprinted and schema contents are only refetched on a change.
 */
export function refreshOntologyDomainFields(): Promise<void> {
  refreshInFlight ??= (async () => {
    const inventory = await fetchHubJson<SchemaListEntry[]>('/api/semantic/schema/list')
    if (fingerprintOf(inventory) === hubFingerprint) return
    const fresh = await loadHub()
    writeHubCache(hubFingerprint, fresh)
    reactiveFields.splice(0, reactiveFields.length, ...fresh.fields)
    reactiveAssets.splice(0, reactiveAssets.length, ...fresh.assets)
    reactiveConstraints.splice(0, reactiveConstraints.length, ...fresh.constraints)
    reactivePartyRoleOptions.splice(0, reactivePartyRoleOptions.length, ...fresh.partyRoleOptions)
  })().finally(() => {
    refreshInFlight = null
  })
  return refreshInFlight
}
