export interface JsonLdReference {
  '@id': string
}

export interface JsonLdTypedValue {
  '@value': string
  /** A compact xsd term, or an external library's exact datatype IRI. */
  '@type': string
}

export interface DcsTemplateMetadata {
  '@id'?: string
  '@type': 'dcs:TemplateMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:templateType': string
  'dcs:customMetaData'?: unknown[]
}

export interface DcsContractMetadata {
  '@id'?: string
  '@type': 'dcs:ContractMetadata'
  'dcs:title'?: string
  'dcs:description'?: string
  'dcs:customMetaData'?: unknown[]
}

/** An XSD datatype declared by a contract field. Every member is a value
 *  space DCS can order, so an ODRL boundary stated over the field has a
 *  defined answer. */
export type XsdDatatype = `xsd:${'string' | 'decimal' | 'integer' | 'boolean' | 'date' | 'dateTime' | 'duration'}`

const XSD = 'http://www.w3.org/2001/XMLSchema#'

/**
 * The XSD datatypes an imported library may declare, each mapped to the
 * compact term a dcs:ContractField carries. A member maps only where the two
 * share a value space: the integer and decimal families keep their numeric
 * order, the string family its lexical order, durations stay durations.
 * Anything else in the XSD namespace is a datatype DCS cannot order — reading
 * it as a string turns "PT6H <= PT24H" into a byte comparison that answers,
 * and answers wrong — so compactXsdDatatype rejects it rather than coercing.
 */
const XSD_TO_COMPACT: Readonly<Record<string, XsdDatatype>> = {
  string: 'xsd:string',
  normalizedString: 'xsd:string',
  token: 'xsd:string',
  language: 'xsd:string',
  Name: 'xsd:string',
  NCName: 'xsd:string',
  NMTOKEN: 'xsd:string',
  anyURI: 'xsd:string',
  hexBinary: 'xsd:string',
  base64Binary: 'xsd:string',
  decimal: 'xsd:decimal',
  double: 'xsd:decimal',
  float: 'xsd:decimal',
  integer: 'xsd:integer',
  int: 'xsd:integer',
  long: 'xsd:integer',
  short: 'xsd:integer',
  byte: 'xsd:integer',
  nonNegativeInteger: 'xsd:integer',
  positiveInteger: 'xsd:integer',
  nonPositiveInteger: 'xsd:integer',
  negativeInteger: 'xsd:integer',
  unsignedLong: 'xsd:integer',
  unsignedInt: 'xsd:integer',
  unsignedShort: 'xsd:integer',
  unsignedByte: 'xsd:integer',
  boolean: 'xsd:boolean',
  date: 'xsd:date',
  dateTime: 'xsd:dateTime',
  dateTimeStamp: 'xsd:dateTime',
  duration: 'xsd:duration',
  dayTimeDuration: 'xsd:duration',
  yearMonthDuration: 'xsd:duration',
}

/**
 * The compact term for a declared datatype IRI (absolute or already compact),
 * or undefined when the IRI names no datatype at all — an rdfs:range that is
 * a class, an sh:class leaf, an absent declaration.
 *
 * Throws for an IRI that IS in the XSD namespace but names a datatype DCS
 * cannot order. Silently degrading it to xsd:string is the failure this
 * guards: the declaration is lost, and every later comparison over the field
 * runs on bytes.
 */
export function compactXsdDatatype(iri: string): XsdDatatype | undefined {
  const local = iri.startsWith(XSD) ? iri.slice(XSD.length) : iri.startsWith('xsd:') ? iri.slice(4) : ''
  if (!local) return undefined
  const compact = XSD_TO_COMPACT[local]
  if (!compact) {
    throw new Error(
      `<${iri}> is an XSD datatype DCS cannot order. Reading it as a string would let a policy boundary over ` +
        'this field compare lexically and answer wrong. Declare a supported datatype, or add this one to ' +
        'XSD_TO_COMPACT and to compareXsdValues in xsd-order.ts.',
    )
  }
  return compact
}

/** A declared contract field referenced by domain data and clause prose. */
export interface DcsContractField {
  '@id': string
  '@type': 'dcs:ContractField'
  'dcs:label': string
  'dcs:datatype': XsdDatatype
  'dcs:shape'?: JsonLdReference
  'dcs:required': boolean
  'dcs:value'?: string | number | boolean | JsonLdTypedValue
  'dcs:valueConstraint'?: import('@template-repository/models/contract-template').SemanticValueConstraint
}

/** The local name of an IRI or compact term — its part after the last
 *  '#', '/' or ':'. */
export function localNameOf(iri: string): string {
  return iri.replace(/^.*[#/:]/, '')
}

/** Serializes a fill as a typed literal carrying the field's declared
 *  datatype — a compact xsd term, or an external library's exact datatype
 *  IRI. The lexical form is a string, so the document carries the exact
 *  token the user agreed to — deterministic across round trips. */
export function typedFieldFill(value: string | number | boolean, datatype: string): JsonLdTypedValue {
  return { '@value': String(value), '@type': datatype }
}

/** Reads a fill back to the editor's scalar, accepting the typed-literal
 *  serialization and bare scalars alike. Returns undefined for an absent
 *  fill. A typed fill converts per the declared datatype — a decimal field's
 *  editor holds a NUMBER — so write and read stay symmetric and draft
 *  dirty-checks never see a phantom change. */
export function fieldFillScalar(
  fill: DcsContractField['dcs:value'],
  datatype?: XsdDatatype,
): string | number | boolean | undefined {
  if (fill === null || fill === undefined) return undefined
  if (typeof fill !== 'object') return fill
  const lexical = fill['@value']
  switch (datatype ?? fill['@type']) {
    case 'xsd:decimal':
    case 'xsd:integer':
      return Number(lexical)
    case 'xsd:boolean':
      return lexical === 'true'
    default:
      return lexical
  }
}

/** A clause references a ContractField only by its @id. */
export type DcsContractFieldRef = JsonLdReference

export type DcsContentSegment = string | DcsContractFieldRef

/** A property value in the contract-data graph: a literal (fixed data), a
 *  typed literal, or a reference to a declared field or another domain
 *  object. */
export type DcsContractDataValue = string | number | boolean | JsonLdTypedValue | JsonLdReference

/** A typed domain object in the contract-data graph. Properties hold
 *  literals, references to declared contract fields (negotiable leaves), or
 *  references to other domain objects (structure, arbitrary depth). */
export type DcsContractDataObject = {
  '@id': string
  '@type': string
} & Record<string, DcsContractDataValue | DcsContractDataValue[] | undefined>

export interface DcsSection {
  '@type': 'dcs:Section'
  '@id': string
  'dcs:title'?: string
}

export interface DcsTextBlock {
  '@type': 'dcs:TextBlock'
  '@id': string
  'dcs:text': string
}

export interface DcsSignatureField {
  '@type': 'dcs:SignatureField'
  'dcs:name': string
  'dcs:label'?: string
}

export interface DcsClause {
  '@type': 'dcs:Clause'
  '@id': string
  'dcs:content': { '@list': DcsContentSegment[] } | string
  'dcs:title'?: string
  'dcs:signatureFields'?: DcsSignatureField[]
}

export type DcsBlock = DcsSection | DcsTextBlock | DcsClause

export interface DcsLayoutNode {
  '@id': string
  '@type'?: 'dcs:LayoutNode'
  'dcs:isRoot'?: boolean
  'dcs:children': { '@list': JsonLdReference[] }
}

export interface DcsDocumentStructure {
  '@id'?: string
  '@type': 'dcs:DocumentStructure'
  'dcs:blocks': { '@list': DcsBlock[] }
  'dcs:layout': { '@list': DcsLayoutNode[] }
}

export interface OdrlConstraint {
  '@type': 'odrl:Constraint'
  'odrl:leftOperand': JsonLdReference
  'odrl:operator': JsonLdReference
  /**
   * The boundary the left operand is checked against: a fixed literal (or list
   * for set operators), or a reference to a RequirementField whose value is
   * agreed during contract negotiation. SRS Appendix C is a template whose
   * spatial and dateTime boundaries (the permitted region, the access deadline)
   * are negotiated field references, resolved to their filled values at
   * enforcement.
   */
  'odrl:rightOperand'?: JsonLdTypedValue | JsonLdReference | (JsonLdTypedValue | JsonLdReference)[]
  /**
   * The unit the boundary is measured in (ODRL IM §2.5), an IRI: a currency
   * concept for a payment amount, a countable unit for a count. Without it a
   * downstream target system cannot tell what the number denominates.
   */
  'odrl:unit'?: JsonLdReference
}

/**
 * An ODRL LogicalConstraint (IM §2.6): a logical operator over an ordered list
 * of constraints. and/andSequence = all hold, or = any holds, xone = exactly
 * one holds; children may themselves be logical (a tree).
 */
export interface OdrlLogicalConstraint {
  '@type': 'odrl:LogicalConstraint'
  'odrl:and'?: { '@list': OdrlConstraintNode[] }
  'odrl:or'?: { '@list': OdrlConstraintNode[] }
  'odrl:xone'?: { '@list': OdrlConstraintNode[] }
  'odrl:andSequence'?: { '@list': OdrlConstraintNode[] }
}

export type OdrlConstraintNode = OdrlConstraint | OdrlLogicalConstraint

export function isAtomicConstraint(node: OdrlConstraintNode): node is OdrlConstraint {
  return node['@type'] === 'odrl:Constraint'
}

/**
 * A Duty nested under a Permission (ODRL IM §2.5): an obligation the assignee
 * must fulfil to exercise the permission. A duty is a *fragment* — it carries
 * its own action and constraints, while the assigner/assignee/target are
 * inherited from the enclosing rule (so, unlike a top-level rule, it declares
 * none of them). A duty may carry a consequence: a further duty that becomes
 * active when the duty itself is not fulfilled.
 */
export interface OdrlDuty {
  '@id'?: string
  '@type': 'odrl:Duty'
  'odrl:action': JsonLdReference | JsonLdReference[]
  /** The clause node this duty is backed by. A nested duty is an odrl:Duty like
   *  any other, so it owes prose too; it is authored inside the clause its
   *  enclosing rule cites, and cites the same block (set on assembly, once the
   *  clause block has an IRI). */
  'dcs:prose'?: JsonLdReference
  'odrl:constraint'?: OdrlConstraintNode[]
  'odrl:consequence'?: OdrlDuty[]
}

export interface OdrlRule {
  '@id': string
  '@type': 'odrl:Duty' | 'odrl:Permission' | 'odrl:Prohibition'
  /**
   * The action(s) the rule governs. A single action is one reference; several
   * actions are an array (ODRL Policy Rule Composition §2.7 — normatively the
   * atomic equivalent is one rule per action).
   */
  'odrl:action': JsonLdReference | JsonLdReference[]
  /** Bound party DIDs for a contract instance (ODRL Agreement); open/placeholder party references for a template (ODRL Offer). */
  'odrl:assigner': JsonLdReference
  'odrl:assignee': JsonLdReference
  /** The contract/data-asset IRI this rule applies to. */
  'odrl:target': JsonLdReference
  /** The human-readable clause node this rule is backed by (required — machine rules operationalize audited prose). */
  'dcs:prose': JsonLdReference
  /** The rule's constraints. A plain list is a conjunction (all hold, ODRL IM
   *  §2.5); a single LogicalConstraint expresses or/xone/andSequence. Nodes may
   *  nest (a constraint tree). */
  'odrl:constraint'?: OdrlConstraintNode[]
  /** Duties the assignee must fulfil to exercise this rule (ODRL IM §2.5 —
   *  meaningful on a Permission). Each is a fragment with its own action and
   *  constraints. */
  'odrl:duty'?: OdrlDuty[]
}

/** The single enclosing ODRL 2.2 policy for a template (Offer) or contract (Agreement). */
export interface OdrlSet {
  '@id': string
  '@type': 'odrl:Offer' | 'odrl:Agreement'
  'odrl:profile': JsonLdReference
  /** Policy-level Duty rules (ODRL 2.2: a Policy carries obligation, never duty — duty nests under a Permission). */
  'odrl:obligation'?: OdrlRule[]
  'odrl:permission'?: OdrlRule[]
  'odrl:prohibition'?: OdrlRule[]
}

export interface DcsDocumentData {
  /** Anchored server-side to the Semantic Hub's versioned context URL; the client never emits it. */
  '@context'?: unknown
  '@type': 'dcs:ContractTemplate' | 'dcs:Contract'
  '@id'?: string
  'dcs:metadata': DcsTemplateMetadata | DcsContractMetadata
  'dcs:documentStructure': DcsDocumentStructure
  /** Typed business objects whose properties bind to declared contract fields. */
  'dcs:contractData': DcsContractDataObject[]
  /** Flat registry of fillable field declarations. */
  'dcs:contractFields': DcsContractField[]
  'dcs:policies': OdrlSet
}

export interface DcsTemplateData extends DcsDocumentData {
  '@type': 'dcs:ContractTemplate'
  'dcs:metadata': DcsTemplateMetadata
  /**
   * The contractual roles the template declares, as party placeholder nodes
   * whose IRIs end `#party-<role>`. Creating a contract binds the originating
   * organization to one of them (backend command/create.go bindOriginatorParty).
   */
  'dcs:parties'?: unknown[]
}

export interface DcsContractData extends DcsDocumentData {
  '@type': 'dcs:Contract'
  'dcs:metadata': DcsContractMetadata | DcsTemplateMetadata
  'dcs:parentContract'?: JsonLdReference
  derivedFromTemplate?: DcsTemplateProvenance
  /**
   * Server-owned parts of the document the editor neither authors nor models:
   * the contract's party nodes (which carry the recorded signatory and Power of
   * Attorney of an applied signature), the AcroForm signature fields naming
   * them, and the pinned shapes graph the document is validated against. The
   * client only carries them — see preserveServerOwnedFields.
   */
  'dcs:parties'?: unknown[]
  'dcs:signatureFields'?: unknown[]
  'sh:shapesGraph'?: unknown
}

/** The source-template node: a prov:wasDerivedFrom edge plus version assertion. */
export interface DcsTemplateProvenance {
  '@id': string
  version?: number
}

export function isDcsSection(block: DcsBlock): block is DcsSection {
  return block['@type'] === 'dcs:Section'
}

export function isDcsClause(block: DcsBlock): block is DcsClause {
  return block['@type'] === 'dcs:Clause'
}

export function isDcsDocumentData(raw: unknown): raw is DcsDocumentData {
  if (typeof raw !== 'object' || raw === null) return false
  const value = raw as Record<string, unknown>
  const policies = value['dcs:policies']
  return (
    (value['@type'] === 'dcs:ContractTemplate' || value['@type'] === 'dcs:Contract') &&
    typeof value['dcs:documentStructure'] === 'object' &&
    Array.isArray(value['dcs:contractData']) &&
    Array.isArray(value['dcs:contractFields']) &&
    // Canonical shape: a single enclosing odrl:Set object.
    // An empty array is still accepted as "no policies yet" (brand-new
    // documents); a non-empty bare-rule array is not.
    (isOdrlSet(policies) || (Array.isArray(policies) && policies.length === 0))
  )
}

function isOdrlSet(value: unknown): value is OdrlSet {
  if (typeof value !== 'object' || value === null) return false
  const type = (value as Record<string, unknown>)['@type']
  return type === 'odrl:Offer' || type === 'odrl:Agreement'
}

export function isDcsTemplateData(raw: unknown): raw is DcsTemplateData {
  return isDcsDocumentData(raw) && raw['@type'] === 'dcs:ContractTemplate'
}
