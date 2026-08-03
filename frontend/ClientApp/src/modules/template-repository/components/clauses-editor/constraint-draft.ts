import {
  isAtomicConstraint,
  type JsonLdReference,
  type JsonLdTypedValue,
  type OdrlConstraint,
  type OdrlConstraintNode,
  type OdrlLogicalConstraint,
} from '@/models/dcs-jsonld'
import { XSD_DURATION } from '@/models/xsd-order'

/**
 * The editor's recursive model of an ODRL constraint (ODRL IM §2.5/§2.6). A
 * node is either an atomic Constraint (leftOperand/operator/rightOperand) or a
 * logical group (a combinator over child nodes, themselves atomic or logical —
 * an arbitrarily deep tree). The rule and every duty share this model, so the
 * full ODRL constraint grammar is authorable everywhere a constraint is.
 */

/** How a group's child constraints combine (ODRL LogicalConstraint IM §2.6). */
export const CONSTRAINT_COMBINATORS = [
  { op: 'and', label: 'ALL must hold' },
  { op: 'or', label: 'ANY may hold' },
  { op: 'xone', label: 'EXACTLY ONE must hold' },
  { op: 'andSequence', label: 'ALL, in sequence' },
] as const
export type ConstraintCombinator = (typeof CONSTRAINT_COMBINATORS)[number]['op']

export type OperandDraftValue = JsonLdReference | JsonLdTypedValue

export interface AtomicDraft {
  kind: 'atomic'
  leftOperand: string
  operator: string
  /** '' = a fixed literal boundary (use `value` or `values`); otherwise a field @id whose
   *  value is agreed during contract negotiation. */
  rightSource: string
  value: string
  values: OperandDraftValue[]
  /** '' = the unit is the fixed IRI in `unit`; otherwise a field @id whose value
   *  — the unit the boundary is measured in — is agreed during negotiation, the
   *  way `rightSource` makes the boundary itself negotiable. A field fills with
   *  a value option's notation ("EUR"), not its concept IRI, so a negotiated
   *  unit and a fixed one are never the same unit to the audit. */
  unitSource: string
  /** '' = no unit; otherwise the IRI of the unit the boundary is measured in
   *  (odrl:unit, e.g. a currency concept). Unused when `unitSource` is set. */
  unit: string
}

export interface GroupDraft {
  kind: 'group'
  combine: ConstraintCombinator
  children: ConstraintNodeDraft[]
}

export type ConstraintNodeDraft = AtomicDraft | GroupDraft

export function isGroupDraft(node: ConstraintNodeDraft): node is GroupDraft {
  return node.kind === 'group'
}

export function newAtomic(leftOperand: string, operator: string): AtomicDraft {
  return {
    kind: 'atomic',
    leftOperand,
    operator,
    rightSource: '',
    value: '',
    values: [],
    unitSource: '',
    unit: '',
  }
}

export function newGroup(): GroupDraft {
  return { kind: 'group', combine: 'and', children: [] }
}

/** The XSD datatype a typed-in boundary literal carries, so a target system
 *  evaluating the constraint can tell a duration from a label and an integral
 *  count from a decimal amount. */
function literalDatatype(value: string): string {
  if (/^-?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(value)) return 'xsd:dateTime'
  if (/^-?\d{4}-\d{2}-\d{2}$/.test(value)) return 'xsd:date'
  if (XSD_DURATION.test(value)) return 'xsd:duration'
  if (value !== '' && !Number.isNaN(Number(value))) {
    return /^[+-]?\d+$/.test(value) ? 'xsd:integer' : 'xsd:decimal'
  }
  return 'xsd:string'
}

export function typed(value: string): JsonLdTypedValue {
  return { '@value': value, '@type': literalDatatype(value) }
}

function isSetOperator(operator: string): boolean {
  return operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf' || operator === 'odrl:isAllOf'
}

function literalRightOperand(value: string, operator: string): JsonLdTypedValue | JsonLdTypedValue[] | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  if (isSetOperator(operator)) {
    // A set operand is an unordered JSON-LD set (bare array): the policy
    // audit expands and matches each member individually, while an @list
    // would reach it as a single nested list value.
    const values = trimmed
      .split(',')
      .map((part) => part.trim())
      .filter(Boolean)
      .map(typed)
    return values.length ? values : undefined
  }
  return typed(trimmed)
}

function nonEmptyOperand(value: OperandDraftValue): boolean {
  return '@id' in value ? !!value['@id'] : value['@value'] !== ''
}

function fixedRightOperand(atomic: AtomicDraft): JsonLdTypedValue | JsonLdReference | OperandDraftValue[] | undefined {
  const values = atomic.values.filter(nonEmptyOperand)
  if (values.length) return isSetOperator(atomic.operator) ? values : values[0]
  return literalRightOperand(atomic.value, atomic.operator)
}

function buildAtomic(atomic: AtomicDraft): OdrlConstraint {
  const constraint: OdrlConstraint = {
    '@type': 'odrl:Constraint',
    'odrl:leftOperand': { '@id': atomic.leftOperand },
    'odrl:operator': { '@id': atomic.operator },
  }
  const right = atomic.rightSource ? { '@id': atomic.rightSource } : fixedRightOperand(atomic)
  if (right !== undefined) constraint['odrl:rightOperand'] = right
  const unit = atomic.unitSource || atomic.unit.trim()
  if (unit) constraint['odrl:unit'] = { '@id': unit }
  return constraint
}

function logicalConstraint(combine: ConstraintCombinator, nodes: OdrlConstraintNode[]): OdrlLogicalConstraint {
  const list = { '@list': nodes }
  switch (combine) {
    case 'or':
      return { '@type': 'odrl:LogicalConstraint', 'odrl:or': list }
    case 'xone':
      return { '@type': 'odrl:LogicalConstraint', 'odrl:xone': list }
    case 'andSequence':
      return { '@type': 'odrl:LogicalConstraint', 'odrl:andSequence': list }
    default:
      return { '@type': 'odrl:LogicalConstraint', 'odrl:and': list }
  }
}

/** Builds one node; a group of a single child collapses to that child, and an
 *  empty (or all-empty) node is dropped (returns undefined). */
function buildNode(node: ConstraintNodeDraft): OdrlConstraintNode | undefined {
  if (node.kind === 'atomic') {
    return node.leftOperand ? buildAtomic(node) : undefined
  }
  const children = node.children.map(buildNode).filter((n): n is OdrlConstraintNode => n !== undefined)
  if (!children.length) return undefined
  const [only] = children
  if (children.length === 1 && only) return only
  return logicalConstraint(node.combine, children)
}

/**
 * Composes a rule's (or duty's) odrl:constraint value from the root group: an
 * ALL root is a plain conjunction array (which may itself contain nested
 * logical nodes, ODRL IM §2.5); any other combinator over more than one node
 * wraps a single LogicalConstraint (IM §2.6).
 */
export function composeConstraintTree(root: GroupDraft): OdrlConstraintNode[] | undefined {
  const children = root.children.map(buildNode).filter((n): n is OdrlConstraintNode => n !== undefined)
  if (!children.length) return undefined
  if (root.combine === 'and' || children.length === 1) return children
  return [logicalConstraint(root.combine, children)]
}

function operandLabel(value: OperandDraftValue): string {
  return '@id' in value ? value['@id'] : String(value['@value'])
}

// A negotiated boundary and a fixed concept boundary both serialize as
// {"@id": …}, and so do a negotiated and a fixed odrl:unit. Only the declared
// field list separates them, so reading a constraint back needs it — without
// it every concept IRI reads as a field reference and reload rewrites the
// contract.
function readAtomic(constraint: OdrlConstraint, fields: ReadonlySet<string>): AtomicDraft {
  const declaredUnit = constraint['odrl:unit']?.['@id'] ?? ''
  const unitSource = fields.has(declaredUnit) ? declaredUnit : ''
  const head = {
    kind: 'atomic' as const,
    leftOperand: constraint['odrl:leftOperand']['@id'],
    operator: constraint['odrl:operator']['@id'],
    unitSource,
    unit: unitSource ? '' : declaredUnit,
  }
  const right = constraint['odrl:rightOperand']
  if (right && !Array.isArray(right) && '@id' in right && fields.has(right['@id'])) {
    return { ...head, rightSource: right['@id'], value: '', values: [] }
  }
  const values: OperandDraftValue[] = Array.isArray(right)
    ? right.map((item) => ({ ...item }))
    : right
      ? [{ ...right }]
      : []
  return { ...head, rightSource: '', value: values.map(operandLabel).join(', '), values }
}

function logicalList(node: OdrlLogicalConstraint, op: ConstraintCombinator): OdrlConstraintNode[] | undefined {
  switch (op) {
    case 'or':
      return node['odrl:or']?.['@list']
    case 'xone':
      return node['odrl:xone']?.['@list']
    case 'andSequence':
      return node['odrl:andSequence']?.['@list']
    default:
      return node['odrl:and']?.['@list']
  }
}

function parseNode(node: OdrlConstraintNode, fields: ReadonlySet<string>): ConstraintNodeDraft | undefined {
  if (isAtomicConstraint(node)) return readAtomic(node, fields)
  return parseLogical(node, fields)
}

function parseLogical(node: OdrlLogicalConstraint, fields: ReadonlySet<string>): GroupDraft | undefined {
  for (const { op } of CONSTRAINT_COMBINATORS) {
    const list = logicalList(node, op)
    if (list) {
      return {
        kind: 'group',
        combine: op,
        children: list
          .map((child) => parseNode(child, fields))
          .filter((n): n is ConstraintNodeDraft => n !== undefined),
      }
    }
  }
  return undefined
}

/** Reads a rule's (or duty's) odrl:constraint back into the editor's root
 *  group: a single LogicalConstraint surfaces its combinator and subtree; a
 *  plain list is an ALL conjunction whose members may themselves be logical.
 *  `declaredFields` are the @ids of the contract fields in scope — what a
 *  reference to a negotiated boundary or unit may name. */
export function parseConstraintTree(nodes: OdrlConstraintNode[], declaredFields: readonly string[]): GroupDraft {
  const fields = new Set(declaredFields)
  const [first] = nodes
  if (nodes.length === 1 && first && !isAtomicConstraint(first)) {
    const group = parseLogical(first, fields)
    if (group) return group
  }
  return {
    kind: 'group',
    combine: 'and',
    children: nodes.map((node) => parseNode(node, fields)).filter((n): n is ConstraintNodeDraft => n !== undefined),
  }
}
