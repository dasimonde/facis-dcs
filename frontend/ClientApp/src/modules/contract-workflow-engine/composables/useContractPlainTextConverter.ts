import type { ContractData, SemanticConditionValue } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import { isNewline, isPlaceholder, isText, parseSegments } from '@template-repository/composables/useClauseTextChips'
import type {
  ApprovedTemplateBlock,
  ClauseBlock,
  DocumentBlock,
  DocumentOutline,
  DocumentOutlineBlock,
  SectionBlock,
  SemanticCondition,
  TextBlock,
} from '@template-repository/models/contract-templace'
import {
  isApprovedTemplateBlock,
  isClauseBlock,
  isMergedApprovedTemplateBlock,
  isSectionBlock,
  isTextBlock,
} from '@template-repository/models/contract-templace'
import {
  getOwnerBlockIdFromMergedBlockId,
  isMergedBlockId,
  isSameTemplateDataRef,
} from '@template-repository/utils/template-data-ref'

const DEFAULT_PLACEHOLDER_TEXT = '__________'
const NEWLINE = '\n'

export type ContractPlainTextInput = Partial<ContractData> | null | undefined

export interface ContractPlainTextSection {
  type: 'section'
  text: string
  level: number
}

export interface ContractPlainTextLine {
  type: 'text'
  text: string
}

export type ContractPlainTextBlock = ContractPlainTextSection | ContractPlainTextLine

export function isSectionPlainTextBlock(block: ContractPlainTextBlock): block is ContractPlainTextSection {
  return block.type === 'section'
}

export function isTextPlainTextBlock(block: ContractPlainTextBlock): block is ContractPlainTextLine {
  return block.type === 'text'
}

interface ContractContext {
  blockMap: Map<string, DocumentBlock>
  outlineMap: Map<string, DocumentOutlineBlock>
  rootBlockIds: string[]
  semanticConditions: SemanticCondition[]
  semanticConditionValues: SemanticConditionValue[]
  subTemplateSnapshots: SubTemplateSnapshot[]
}

interface PlainTextWriter {
  addSection: (text: string, level: number) => void
  addText: (text: string) => void
  breakLine: () => void
  breakLineIfOpen: () => void
  addBoundarySpaceIfNeeded: (sourceText: string) => void
  toBlocks: () => ContractPlainTextBlock[]
}

function createContractContext(contractData: ContractPlainTextInput): ContractContext {
  const documentOutline: DocumentOutline = contractData?.documentOutline ?? []
  const documentBlocks: DocumentBlock[] = contractData?.documentBlocks ?? []

  return {
    blockMap: new Map(documentBlocks.map((block) => [block.blockId, block])),
    outlineMap: new Map(documentOutline.map((node) => [node.blockId, node])),
    rootBlockIds: documentOutline.find((node) => node.isRoot)?.children ?? [],
    semanticConditions: contractData?.semanticConditions ?? [],
    semanticConditionValues: contractData?.semanticConditionValues ?? [],
    subTemplateSnapshots: contractData?.subTemplateSnapshots ?? [],
  }
}

function createPlainTextWriter(): PlainTextWriter {
  const blocks: ContractPlainTextBlock[] = []
  let currentLine = ''
  let hasOpenLine = false

  function addSection(text: string, level: number): void {
    breakLineIfOpen()
    blocks.push({
      type: 'section',
      text,
      level,
    })
  }

  function addText(text: string): void {
    currentLine += text
    hasOpenLine = true
  }

  function breakLine(): void {
    blocks.push({
      type: 'text',
      text: currentLine,
    })
    currentLine = ''
    hasOpenLine = false
  }

  function breakLineIfOpen(): void {
    if (!hasOpenLine) return
    breakLine()
  }

  function addBoundarySpaceIfNeeded(sourceText: string): void {
    if (!hasOpenLine) return
    if (!sourceText || sourceText.length === 0) return
    if (sourceText.endsWith(NEWLINE)) return
    const lastChar = sourceText.charAt(sourceText.length - 1)
    if (!/[.,!?;:]/.test(lastChar)) return
    if (currentLine.endsWith(' ')) return
    currentLine += ' '
  }

  function toBlocks(): ContractPlainTextBlock[] {
    breakLineIfOpen()
    return blocks
  }

  return {
    addSection,
    addText,
    breakLine,
    breakLineIfOpen,
    addBoundarySpaceIfNeeded,
    toBlocks,
  }
}

function writeBlockAsPlainText(cxt: ContractContext, blockId: string, level: number, writer: PlainTextWriter): void {
  const block = cxt.blockMap.get(blockId)
  if (!block) return
  if (isSectionBlock(block)) writeSectionBlock(cxt, block, level, writer)
  else if (isTextBlock(block)) writeTextBlock(block, writer)
  else if (isClauseBlock(block)) writeClauseBlock(cxt, block, writer)
  else if (isApprovedTemplateBlock(block)) writeApprovedTemplateBlock(cxt, block, level, writer)
  else if (isMergedApprovedTemplateBlock(block)) writeChildBlocks(cxt, block.blockId, level, writer)
}

function writeSectionBlock(cxt: ContractContext, block: SectionBlock, level: number, writer: PlainTextWriter): void {
  writer.addSection(block.title ?? block.text ?? '', level)
  writeChildBlocks(cxt, block.blockId, level + 1, writer)
}

function writeTextBlock(block: TextBlock, writer: PlainTextWriter): void {
  const lines = (block.text ?? '').split(NEWLINE)
  for (let index = 0; index < lines.length; index += 1) {
    writer.addText(lines[index] ?? '')
    if (index < lines.length - 1) writer.breakLine()
  }
  writer.addBoundarySpaceIfNeeded(block.text ?? '')
}

function writeClauseBlock(cxt: ContractContext, block: ClauseBlock, writer: PlainTextWriter): void {
  const semanticConditions = getSemanticConditionsForClauseBlock(block.blockId, cxt)
  writeClauseText(block.text ?? '', block.blockId, semanticConditions, cxt.semanticConditionValues, writer)
  writer.addBoundarySpaceIfNeeded(block.text ?? '')
}

function writeApprovedTemplateBlock(
  cxt: ContractContext,
  block: ApprovedTemplateBlock,
  level: number,
  writer: PlainTextWriter,
): void {
  writer.breakLineIfOpen()
  const snapshot = cxt.subTemplateSnapshots.find((item) =>
    isSameTemplateDataRef(
      {
        templateId: item.did,
        version: item.version,
        document_number: item.document_number,
      },
      {
        templateId: block.templateId,
        version: block.version,
        document_number: block.document_number,
      },
    ),
  )

  if (snapshot?.template_data) {
    const snapshotContext = createContractContext({
      documentOutline: snapshot.template_data.documentOutline ?? [],
      documentBlocks: snapshot.template_data.documentBlocks ?? [],
      semanticConditions: snapshot.template_data.semanticConditions ?? [],
      semanticConditionValues: cxt.semanticConditionValues,
      subTemplateSnapshots: cxt.subTemplateSnapshots,
    })

    for (const childId of snapshotContext.rootBlockIds) {
      writeBlockAsPlainText(snapshotContext, childId, level, writer)
    }
  }

  writeChildBlocks(cxt, block.blockId, level, writer)
}

function writeChildBlocks(cxt: ContractContext, parentBlockId: string, level: number, writer: PlainTextWriter): void {
  const childIds = cxt.outlineMap.get(parentBlockId)?.children ?? []
  for (const childId of childIds) {
    writeBlockAsPlainText(cxt, childId, level, writer)
  }
}

function writeClauseText(
  text: string,
  blockId: string,
  semanticConditions: SemanticCondition[],
  semanticConditionValues: SemanticConditionValue[],
  writer: PlainTextWriter,
): void {
  const segments = parseSegments(text ?? '', semanticConditions)

  for (const segment of segments) {
    if (isText(segment)) {
      writer.addText(segment.value)
      continue
    }

    if (isPlaceholder(segment)) {
      const parameterValue = semanticConditionValues.find(
        (item) =>
          item.blockId === blockId &&
          item.conditionId === segment.conditionId &&
          item.parameterName === segment.parameterName,
      )?.parameterValue
      writer.addText(parameterValue == null ? DEFAULT_PLACEHOLDER_TEXT : String(parameterValue))
      continue
    }

    if (isNewline(segment)) writer.breakLine()
  }
}

function getSemanticConditionsForClauseBlock(blockId: string, cxt: ContractContext): SemanticCondition[] {
  if (!isMergedBlockId(blockId)) return cxt.semanticConditions
  const ownerBlockId = getOwnerBlockIdFromMergedBlockId(blockId)
  if (!ownerBlockId) return cxt.semanticConditions

  const ownerBlock = cxt.blockMap.get(ownerBlockId)
  if (!ownerBlock || !isMergedApprovedTemplateBlock(ownerBlock)) return cxt.semanticConditions

  const snapshot = cxt.subTemplateSnapshots.find((item) =>
    isSameTemplateDataRef(
      {
        templateId: item.did,
        version: item.version,
        document_number: item.document_number,
      },
      {
        templateId: ownerBlock.templateId,
        version: ownerBlock.version,
        document_number: ownerBlock.document_number,
      },
    ),
  )

  return snapshot?.template_data?.semanticConditions ?? cxt.semanticConditions
}

export function useContractPlainTextConverter() {
  function convertContractToPlainTextBlocks(contractData: ContractPlainTextInput): ContractPlainTextBlock[] {
    const context = createContractContext(contractData)
    if (context.rootBlockIds.length === 0) return []

    const writer = createPlainTextWriter()
    for (const childId of context.rootBlockIds) {
      writeBlockAsPlainText(context, childId, 1, writer)
    }

    return writer.toBlocks()
  }

  return { convertContractToPlainTextBlocks }
}
