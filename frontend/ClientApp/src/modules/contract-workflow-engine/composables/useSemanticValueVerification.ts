import { normalizeNumberInput } from '@template-repository/utils/number-format'
import { resolveAllowedValues } from '@template-repository/utils/value-constraint-catalog'
import { compareXsdValues } from '@/models/xsd-order'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsBlock, DcsClause } from '@/models/dcs-jsonld'
import type { SemanticCondition, SemanticValueConstraint } from '@template-repository/models/contract-template'

export interface VerificationResult {
  isValid: boolean
  errors: {
    blockId: string
    conditionId: string
    parameterName: string
    message: string
  }[]
}

export type VerificationError = VerificationResult['errors'][number]

/** The error to mark a rendered placeholder with, if any.
 *
 *  A value is keyed by its placeholder @id (conditionId), block-agnostically,
 *  and the same placeholder may be referenced from more than one clause — so a
 *  value-level error carries no blockId and belongs to every block rendering
 *  that placeholder. An error that does name a block (a required parameter with
 *  no value at all, which only that clause states) stays scoped to it. */
export function findVerificationError(
  result: VerificationResult | null | undefined,
  blockId: string,
  conditionId: string,
  parameterName: string,
): VerificationError | null {
  if (!result) return null
  return (
    result.errors.find(
      (error) =>
        error.conditionId === conditionId &&
        error.parameterName === parameterName &&
        (!error.blockId || error.blockId === blockId),
    ) ?? null
  )
}

function clauseConditionIds(clause: DcsClause, semanticConditions: SemanticCondition[]): string[] {
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  const fieldIds = new Set<string>()
  for (const seg of content['@list']) {
    if (typeof seg !== 'string') fieldIds.add(seg['@id'])
  }
  const conditionIds = new Set<string>()
  for (const cond of semanticConditions) {
    if (cond.parameters.some((p) => p.fieldId && fieldIds.has(p.fieldId))) {
      conditionIds.add(cond.conditionId)
    }
  }
  return [...conditionIds]
}

export function hasConditionParameterForValue(
  conditionValue: SemanticConditionValue,
  blocks: DcsBlock[],
  semanticConditions: SemanticCondition[],
): boolean {
  // A filled value is keyed by its placeholder @id (conditionId), not by the
  // referencing block — its blockId is intentionally empty. It stays valid as
  // long as some clause references that placeholder and the matching condition
  // declares the parameter. Looking the block up by blockId (== '') matched
  // nothing, so the cleanup watcher dropped every value, emptied the store and
  // flipped changedContractData true, disabling Submit.
  const referenced = blocks.some(
    (block) =>
      block['@type'] === 'dcs:Clause' &&
      clauseConditionIds(block, semanticConditions).includes(conditionValue.conditionId),
  )
  if (!referenced) return false

  const matchedCondition = semanticConditions.find((condition) => condition.conditionId === conditionValue.conditionId)
  if (!matchedCondition) return false
  return matchedCondition.parameters.some((parameter) => parameter.parameterName === conditionValue.parameterName)
}

export function useSemanticValueVerification() {
  function validateParameterType(value: string | number | boolean, type: string): boolean {
    switch (type) {
      case 'string':
        return typeof value === 'string'
      case 'enum':
        return typeof value === 'string'
      case 'boolean':
        return typeof value === 'boolean'
      case 'integer':
        return typeof value === 'number' && Number.isInteger(value)
      case 'decimal':
        return typeof value === 'number' && !Number.isNaN(value)
      case 'date':
        return typeof value === 'string' && !isNaN(Date.parse(value))
      default:
        return false
    }
  }

  function validateValueConstraint(
    value: string | number | boolean,
    constraint?: SemanticValueConstraint,
  ): string | null {
    if (!constraint) return null
    const allowedValues = resolveAllowedValues(constraint)
    if (allowedValues.length) {
      if (typeof value !== 'string' || !allowedValues.includes(value)) {
        return `Expected one of: ${allowedValues.join(', ')}.`
      }
    }
    if (constraint.pattern) {
      if (typeof value !== 'string' || !new RegExp(constraint.pattern).test(value)) {
        return `Expected format ${constraint.allowedValuesRef ?? constraint.format ?? constraint.pattern}.`
      }
    }
    if (typeof value === 'number') {
      if (constraint.min !== undefined && value < constraint.min) {
        return `Expected a value greater than or equal to ${constraint.min}.`
      }
      if (constraint.max !== undefined && value > constraint.max) {
        return `Expected a value less than or equal to ${constraint.max}.`
      }
    }
    return null
  }

  function verifySemanticValue(
    semanticConditions: SemanticCondition[],
    semanticConditionValues: SemanticConditionValue[],
    blocks: DcsBlock[],
  ): VerificationResult {
    const errors: VerificationResult['errors'] = []
    const boundaries = currentFieldValues(semanticConditions, semanticConditionValues)
    let isValid = false
    blocks.forEach((b) => {
      if (b['@type'] !== 'dcs:Clause') return
      const clause = b
      const conditions = semanticConditions
      const conditionIds = clauseConditionIds(clause, conditions)
      conditionIds.forEach((cId) => {
        const condition = conditions.find((c) => c.conditionId === cId)
        if (!condition) return
        condition.parameters.forEach((p) => {
          if (!p.isRequired) return
          const parameterName = p.parameterName
          const isValueExist = semanticConditionValues.find(
            // A value is keyed by its placeholder @id (conditionId), not the
            // referencing block — see PreviewClauseBlock / applyInlineSemanticValues.
            (conditionValue) => conditionValue.conditionId === cId && conditionValue.parameterName === parameterName,
          )
          if (!isValueExist) {
            errors.push({
              blockId: clause['@id'],
              conditionId: cId,
              parameterName: parameterName,
              message: `"${parameterName}" is required but has no value.`,
            })
          }
        })
      })
    })

    semanticConditionValues.forEach((value) => {
      const conditions = semanticConditions
      const fieldName = value.parameterName || 'this field'
      const condition = conditions.find((cond) => cond.conditionId === value.conditionId)
      if (!condition) {
        errors.push({
          blockId: value.blockId,
          conditionId: value.conditionId,
          parameterName: value.parameterName,
          message: 'Semantic rule not found.',
        })
        return
      }
      const parameter = condition.parameters.find((param) => param.parameterName === value.parameterName)
      if (!parameter) {
        errors.push({
          blockId: value.blockId,
          conditionId: value.conditionId,
          parameterName: value.parameterName,
          message: `"${fieldName}" is not defined in the selected semantic rule.`,
        })
        return
      }
      if (parameter.isRequired && (value.parameterValue === undefined || value.parameterValue === null)) {
        errors.push({
          blockId: value.blockId,
          conditionId: value.conditionId,
          parameterName: value.parameterName,
          message: `"${fieldName}" is required but has no value.`,
        })
        return
      }
      if (value.parameterValue !== undefined && value.parameterValue !== null) {
        const isTypeValid = validateParameterType(value.parameterValue, parameter.type)
        if (!isTypeValid) {
          errors.push({
            blockId: value.blockId,
            conditionId: value.conditionId,
            parameterName: value.parameterName,
            message: `"${fieldName}" has an invalid value type. Expected ${parameter.type}.`,
          })
          return
        }
        const constraintError = validateValueConstraint(value.parameterValue, parameter.valueConstraint)
        if (constraintError) {
          errors.push({
            blockId: value.blockId,
            conditionId: value.conditionId,
            parameterName: value.parameterName,
            message: `"${fieldName}" has an invalid value. ${constraintError}`,
          })
          return
        }
        const operatorError = validateParameterOperators(value.parameterValue, parameter.operators ?? [], boundaries)
        if (operatorError) {
          errors.push({
            blockId: value.blockId,
            conditionId: value.conditionId,
            parameterName: value.parameterName,
            message: `"${fieldName}" violates an ODRL obligation. ${operatorError}`,
          })
          return
        }
      }
    })
    if (errors.length === 0) {
      isValid = true
    }
    return { isValid, errors }
  }

  return { verifySemanticValue, hasConditionParameterForValue }
}

function validateParameterOperators(
  value: string | number | boolean,
  operators: { operate: string; targets: unknown[]; targetRefs?: string[] }[],
  boundaries: Map<string, string | number | boolean>,
): string | null {
  for (const operator of operators) {
    // A boundary that references a contract field is bound to whatever that
    // field currently holds — the value being typed into the same form,
    // falling back to the one the document was loaded with. A reference this
    // form cannot resolve (an unfilled field, a concept IRI) states no bound
    // the editor can check, and is left to the server-side policy audit;
    // comparing anyway reported every such boundary as violated.
    const referenced = (operator.targetRefs ?? []).map((fieldId) => boundaries.get(fieldId))
    if (referenced.some((bound) => bound === undefined)) continue
    const operands = [...operator.targets, ...referenced]
    if (!operands.length) continue
    const target = operator.operate === 'odrl:isAnyOf' || operator.operate === 'odrl:isNoneOf' ? operands : operands[0]
    if (!compareOperator(value, operator.operate, target)) {
      return `Expected ${formatOperator(operator.operate)} ${String(target)}.`
    }
  }
  return null
}

/** What each contract field currently holds, keyed by its @id: the value being
 *  edited wins over the one the document was loaded with. A condition is keyed
 *  by the field it surfaces, so a filled value's conditionId is that @id. */
function currentFieldValues(
  semanticConditions: SemanticCondition[],
  semanticConditionValues: SemanticConditionValue[],
): Map<string, string | number | boolean> {
  const values = new Map<string, string | number | boolean>()
  for (const condition of semanticConditions) {
    for (const parameter of condition.parameters) {
      const fieldId = parameter.fieldId ?? condition.conditionId
      const value = parameter.value
      if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
        values.set(fieldId, value)
      }
    }
  }
  for (const filled of semanticConditionValues) {
    if (filled.parameterValue !== undefined && filled.parameterValue !== null) {
      values.set(filled.conditionId, filled.parameterValue)
    }
  }
  return values
}

function compareOperator(value: string | number | boolean, operator: string, target: unknown): boolean {
  switch (operator) {
    case 'odrl:eq':
      return value === coerceTarget(target, value)
    case 'odrl:neq':
      return value !== coerceTarget(target, value)
    case 'odrl:isAnyOf':
      return operatorTargetsContain(value, target)
    case 'odrl:isNoneOf':
      return !operatorTargetsContain(value, target)
    case 'odrl:gt':
      return compareOrdered(value, target, (left, right) => left > right)
    case 'odrl:gteq':
      return compareOrdered(value, target, (left, right) => left >= right)
    case 'odrl:lt':
      return compareOrdered(value, target, (left, right) => left < right)
    case 'odrl:lteq':
      return compareOrdered(value, target, (left, right) => left <= right)
    case 'odrl:hasPart':
      return typeof value === 'string' && typeof target === 'string' && value.includes(target)
    case 'dcs:matchesRegex':
      return typeof value === 'string' && typeof target === 'string' && new RegExp(target).test(value)
    default:
      return true
  }
}

/** Orders an operand pair inside the value space the two share — numeric,
 *  xsd:duration, or instant — never by bytes: "PT6H" sorts after "PT24H"
 *  lexically, so a byte comparison reads a six-hour window as breaching a
 *  24-hour bound. A pair sharing no ordering states no bound this form can
 *  clear, and is reported rather than waved through. */
function compareOrdered(
  value: string | number | boolean,
  target: unknown,
  compare: (left: number, right: number) => boolean,
): boolean {
  const ordering = compareXsdValues(value, target)
  if (ordering === null) return false
  return compare(ordering, 0)
}

function coerceTarget(target: unknown, value: string | number | boolean): unknown {
  if (typeof value === 'number') {
    return typeof target === 'number' ? target : Number(normalizeNumberInput(String(target)))
  }
  if (typeof value === 'boolean') return typeof target === 'boolean' ? target : target === 'true'
  return target
}

function formatOperator(operator: string): string {
  switch (operator) {
    case 'odrl:eq':
      return '='
    case 'odrl:neq':
      return '!='
    case 'odrl:isAnyOf':
      return 'one of'
    case 'odrl:isNoneOf':
      return 'none of'
    case 'odrl:gt':
      return '>'
    case 'odrl:gteq':
      return '>='
    case 'odrl:lt':
      return '<'
    case 'odrl:lteq':
      return '<='
    case 'odrl:hasPart':
      return 'contains'
    case 'dcs:matchesRegex':
      return 'matches'
    default:
      return operator
  }
}

function operatorTargetsContain(value: string | number | boolean, target: unknown): boolean {
  const targets = Array.isArray(target) ? target : [target]
  return targets.some((item) => coerceTarget(item, value) === value)
}
