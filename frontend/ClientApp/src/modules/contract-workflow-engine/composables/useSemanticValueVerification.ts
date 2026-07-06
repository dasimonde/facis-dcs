import type { SemanticConditionValue } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { SemanticCondition, SemanticValueConstraint } from '@/modules/template-repository/models/contract-template'
import type { DcsBlock, DcsClause } from '@/models/dcs-jsonld'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'
import { isDcsMergedApprovedTemplate } from '@template-repository/store/dcsDraftStore'
import {
  getOwnerBlockIdFromMergedBlockId,
  isMergedBlockId,
  isSameTemplateDataRef,
} from '@template-repository/utils/template-data-ref'
import { normalizeNumberInput } from '@template-repository/utils/number-format'
import { resolveAllowedValues } from '@template-repository/utils/value-constraint-catalog'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'

export interface VerificationResult {
  isValid: boolean
  errors: {
    blockId: string
    conditionId: string
    parameterName: string
    message: string
  }[]
}

interface subTemplateSemanticCondition {
  templateId: string
  version: number
  document_number?: string
  semanticConditions: SemanticCondition[]
}

function clauseConditionIds(clause: DcsClause, semanticConditions: SemanticCondition[]): string[] {
  const content = clause['dcs:content']
  if (typeof content === 'string') return []
  const fieldIds = new Set<string>()
  for (const seg of content['@list']) {
    if (typeof seg !== 'string') fieldIds.add(seg['dcs:bindsTo']['@id'])
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
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  semanticConditions: SemanticCondition[],
  subTemplateSnapshots: SubTemplateSnapshot[],
): boolean {
  const block = blocks.find((b) => b['@id'] === conditionValue.blockId)
  if (block?.['@type'] !== 'dcs:Clause') return false
  const clause = block
  const availableConditions = getConditionsByBlockId(
    conditionValue.blockId,
    blocks,
    semanticConditions,
    subTemplateSnapshots,
  )
  const condIds = clauseConditionIds(clause, availableConditions)
  if (!condIds.includes(conditionValue.conditionId)) return false

  const matchedCondition = availableConditions.find((condition) => condition.conditionId === conditionValue.conditionId)
  if (!matchedCondition) return false
  return matchedCondition.parameters.some((parameter) => parameter.parameterName === conditionValue.parameterName)
}

function getConditionsByBlockId(
  blockId: string,
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  semanticConditions: SemanticCondition[],
  subTemplateSnapshots: SubTemplateSnapshot[],
): SemanticCondition[] {
  let conditions = semanticConditions
  if (!isMergedBlockId(blockId)) return conditions

  const ownerBlockId = getOwnerBlockIdFromMergedBlockId(blockId)
  if (!ownerBlockId) return conditions
  const mergedBlock = blocks.find((b) => b['@id'] === ownerBlockId)
  if (!mergedBlock || !isDcsMergedApprovedTemplate(mergedBlock)) return conditions

  const matchedSnapshot = subTemplateSnapshots.find((snapshot) =>
    isSameTemplateDataRef(
      { templateId: snapshot.did, version: snapshot.version, document_number: snapshot.document_number },
      {
        templateId: mergedBlock['dcs:templateDid'],
        version: mergedBlock['dcs:version'],
        document_number: mergedBlock['dcs:documentNumber'],
      },
    ),
  )
  if (matchedSnapshot?.template_data) conditions = getSemanticConditionsFromTemplateData(matchedSnapshot.template_data)
  return conditions
}

export function useSemanticValueVerification() {
  function getConditions(
    blockId: string,
    blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
    semanticConditions: SemanticCondition[],
    subTemplateSemanticConditions: subTemplateSemanticCondition[],
  ): SemanticCondition[] {
    let conditions = semanticConditions
    if (!isMergedBlockId(blockId)) return conditions
    const ownerBlockId = getOwnerBlockIdFromMergedBlockId(blockId)
    const mergedBlock = ownerBlockId ? blocks.find((b) => b['@id'] === ownerBlockId) : undefined
    if (mergedBlock && isDcsMergedApprovedTemplate(mergedBlock)) {
      const mergedBlockRef = {
        templateId: mergedBlock['dcs:templateDid'],
        version: mergedBlock['dcs:version'],
        document_number: mergedBlock['dcs:documentNumber'],
      }
      const c = subTemplateSemanticConditions.find((c) => isSameTemplateDataRef(c, mergedBlockRef))
      if (c) conditions = c.semanticConditions
    }
    return conditions
  }

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
    subTemplateSemanticConditions: subTemplateSemanticCondition[],
    semanticConditionValues: SemanticConditionValue[],
    blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  ): VerificationResult {
    const errors: VerificationResult['errors'] = []
    let isValid = false
    blocks.forEach((b) => {
      if (b['@type'] !== 'dcs:Clause') return
      const clause = b
      const conditions = getConditions(clause['@id'], blocks, semanticConditions, subTemplateSemanticConditions)
      const conditionIds = clauseConditionIds(clause, conditions)
      conditionIds.forEach((cId) => {
        const condition = conditions.find((c) => c.conditionId === cId)
        if (!condition) return
        condition.parameters.forEach((p) => {
          if (!p.isRequired) return
          const parameterName = p.parameterName
          const isValueExist = semanticConditionValues.find(
            (conditionValue) =>
              conditionValue.blockId === clause['@id'] &&
              conditionValue.conditionId === cId &&
              conditionValue.parameterName === parameterName,
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
      const conditions = getConditions(value.blockId, blocks, semanticConditions, subTemplateSemanticConditions)
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
        const operatorError = validateParameterOperators(value.parameterValue, parameter.operators ?? [])
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
  operators: { operate: string; targets: unknown[] }[],
): string | null {
  for (const operator of operators) {
    const target =
      operator.operate === 'odrl:isAnyOf' || operator.operate === 'odrl:isNoneOf'
        ? operator.targets
        : operator.targets?.[0]
    if (!compareOperator(value, operator.operate, target)) {
      return `Expected ${formatOperator(operator.operate)} ${String(target)}.`
    }
  }
  return null
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

function compareOrdered(
  value: string | number | boolean,
  target: unknown,
  compare: (left: number, right: number) => boolean,
): boolean {
  const left = orderedValue(value)
  const right = orderedValue(target)
  if (left === null || right === null) return false
  return compare(left, right)
}

function orderedValue(value: unknown): number | null {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'string') {
    const number = Number(normalizeNumberInput(value))
    if (Number.isFinite(number)) return number
    const date = Date.parse(value)
    return Number.isNaN(date) ? null : date
  }
  return null
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
