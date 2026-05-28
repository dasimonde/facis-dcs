import type { SemanticConditionValue } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import {
  isClauseBlock,
  isMergedApprovedTemplateBlock,
  type DocumentBlock,
  type SemanticCondition,
} from '@/modules/template-repository/models/contract-templace'
import {
  getOwnerBlockIdFromMergedBlockId,
  isMergedBlockId,
  isSameTemplateDataRef,
} from '@template-repository/utils/template-data-ref'

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

export function hasConditionParameterForValue(
  conditionValue: SemanticConditionValue,
  documentBlocks: DocumentBlock[],
  semanticConditions: SemanticCondition[],
  subTemplateSnapshots: SubTemplateSnapshot[],
): boolean {
  const clauseBlock = documentBlocks.find((block) => block.blockId === conditionValue.blockId)
  if (!clauseBlock || !isClauseBlock(clauseBlock)) return false
  if (!clauseBlock.conditionIds.includes(conditionValue.conditionId)) return false

  const availableConditions = getConditionsByBlockId(
    conditionValue.blockId,
    documentBlocks,
    semanticConditions,
    subTemplateSnapshots,
  )
  const matchedCondition = availableConditions.find((condition) => condition.conditionId === conditionValue.conditionId)
  if (!matchedCondition) return false
  return matchedCondition.parameters.some((parameter) => parameter.parameterName === conditionValue.parameterName)
}

function getConditionsByBlockId(
  blockId: string,
  documentBlocks: DocumentBlock[],
  semanticConditions: SemanticCondition[],
  subTemplateSnapshots: SubTemplateSnapshot[],
): SemanticCondition[] {
  let conditions = semanticConditions
  if (!isMergedBlockId(blockId)) return conditions

  const ownerBlocId = getOwnerBlockIdFromMergedBlockId(blockId)
  if (!ownerBlocId) return conditions
  const mergedBlock = documentBlocks.find((block) => block.blockId === ownerBlocId)
  if (!mergedBlock || !isMergedApprovedTemplateBlock(mergedBlock)) return conditions

  const matchedSnapshot = subTemplateSnapshots.find((snapshot) =>
    isSameTemplateDataRef(
      {
        templateId: snapshot.did,
        version: snapshot.version,
        document_number: snapshot.document_number,
      },
      {
        templateId: mergedBlock.templateId,
        version: mergedBlock.version,
        document_number: mergedBlock.document_number,
      },
    ),
  )
  if (matchedSnapshot?.template_data?.semanticConditions) {
    conditions = matchedSnapshot.template_data.semanticConditions
  }
  return conditions
}

export function useSemanticValueVerification() {
  function getConditions(
    blockId: string,
    documentBlocks: DocumentBlock[],
    semanticConditions: SemanticCondition[],
    subTemplateSemanticConditions: subTemplateSemanticCondition[],
  ): SemanticCondition[] {
    let conditions = semanticConditions
    if (!isMergedBlockId(blockId)) return conditions
    const ownerBlockId = getOwnerBlockIdFromMergedBlockId(blockId)
    const mergedBlock = ownerBlockId ? documentBlocks.find((b) => b.blockId === ownerBlockId) : undefined
    if (mergedBlock && isMergedApprovedTemplateBlock(mergedBlock)) {
      const mergedBlockRef = {
        templateId: mergedBlock.templateId,
        version: mergedBlock.version,
        document_number: mergedBlock.document_number,
      }
      const c = subTemplateSemanticConditions.find((c) => {
        return isSameTemplateDataRef(c, mergedBlockRef)
      })
      if (c) conditions = c.semanticConditions
    }
    return conditions
  }
  function validateParameterType(value: string | number, type: string): boolean {
    switch (type) {
      case 'string':
        return typeof value === 'string'
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

  function verifySemanticValue(
    semanticConditions: SemanticCondition[],
    subTemplateSemanticConditions: subTemplateSemanticCondition[],
    semanticConditionValues: SemanticConditionValue[],
    documentBlocks: DocumentBlock[],
  ): VerificationResult {
    const errors: VerificationResult['errors'] = []
    let isValid = false
    documentBlocks.forEach((b) => {
      if (!isClauseBlock(b)) return
      const conditions = getConditions(b.blockId, documentBlocks, semanticConditions, subTemplateSemanticConditions)
      const conditionIds = b.conditionIds ?? []
      conditionIds.forEach((cId) => {
        const condition = conditions.find((c) => c.conditionId === cId)
        if (!condition) return
        condition.parameters.forEach((p) => {
          if (!p.isRequired) return
          const parameterName = p.parameterName
          const isValueExist = semanticConditionValues.find(
            (conditionValue) =>
              conditionValue.blockId === b.blockId &&
              conditionValue.conditionId === cId &&
              conditionValue.parameterName === parameterName,
          )
          if (!isValueExist) {
            errors.push({
              blockId: b.blockId,
              conditionId: cId,
              parameterName: parameterName,
              message: `"${parameterName}" is required but has no value.`,
            })
          }
        })
      })
    })

    semanticConditionValues.forEach((value) => {
      const conditions = getConditions(value.blockId, documentBlocks, semanticConditions, subTemplateSemanticConditions)
      const fieldName = value.parameterName || 'this field'
      const condition = conditions.find((cond) => cond.conditionId === value.conditionId)
      // check if the condition exists, if not, it's an error
      if (!condition) {
        errors.push({
          blockId: value.blockId,
          conditionId: value.conditionId,
          parameterName: value.parameterName,
          message: 'Semantic rule not found.',
        })
        return
      }
      // check if the parameter exists in the condition, if not, it's an error
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
      // check if the parameter value is provided, if the parameter is required, it's an error if not provided
      if (parameter.isRequired && (value.parameterValue === undefined || value.parameterValue === null)) {
        errors.push({
          blockId: value.blockId,
          conditionId: value.conditionId,
          parameterName: value.parameterName,
          message: `"${fieldName}" is required but has no value.`,
        })
        return
      }
      // check if the parameter value type matches the parameter type, if not, it's an error
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
      }
    })
    if (errors.length === 0) {
      isValid = true
    }
    return { isValid, errors }
  }

  return { verifySemanticValue, hasConditionParameterForValue }
}
