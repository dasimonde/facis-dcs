import {
  isNewline,
  isPlaceholder,
  isText,
  parseSegmentsFromContent,
} from '@template-repository/composables/useClauseTextChips'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import {
  collectDeclaredRequirements,
  fromDocumentSemanticValues,
} from '@contract-workflow-engine/utils/semantic-condition-values'
import { isDcsDocumentData } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsBlock, DcsClause, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { SemanticCondition } from '@template-repository/models/contract-template'

const DEFAULT_PLACEHOLDER_TEXT = '__________'
const NEWLINE = '\n'

export type ContractPlainTextInput = unknown

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
  blockMap: Map<string, DcsBlock>
  layoutMap: Map<string, DcsLayoutNode>
  rootChildIds: string[]
  semanticConditions: SemanticCondition[]
  semanticConditionValues: SemanticConditionValue[]
}

interface PlainTextWriter {
  addSection: (text: string, level: number) => void
  addText: (text: string) => void
  breakLine: () => void
  breakLineIfOpen: () => void
  addBoundarySpaceIfNeeded: (sourceText: string) => void
  toBlocks: () => ContractPlainTextBlock[]
}

function createContractContext(
  blocks: DcsBlock[],
  layout: DcsLayoutNode[],
  semanticConditions: SemanticCondition[],
  semanticConditionValues: SemanticConditionValue[],
): ContractContext {
  const root = layout.find((n) => n['dcs:isRoot'])
  const rootChildIds = root ? root['dcs:children']['@list'].map((r) => r['@id']) : []
  return {
    blockMap: new Map(blocks.map((b) => [b['@id'], b])),
    layoutMap: new Map(layout.map((n) => [n['@id'], n])),
    rootChildIds,
    semanticConditions,
    semanticConditionValues,
  }
}

function createPlainTextWriter(): PlainTextWriter {
  const blocks: ContractPlainTextBlock[] = []
  let currentLine = ''
  let hasOpenLine = false

  function addSection(text: string, level: number): void {
    breakLineIfOpen()
    blocks.push({ type: 'section', text, level })
  }

  function addText(text: string): void {
    currentLine += text
    hasOpenLine = true
  }

  function breakLine(): void {
    blocks.push({ type: 'text', text: currentLine })
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

  return { addSection, addText, breakLine, breakLineIfOpen, addBoundarySpaceIfNeeded, toBlocks }
}

function writeBlockAsPlainText(cxt: ContractContext, blockId: string, level: number, writer: PlainTextWriter): void {
  const block = cxt.blockMap.get(blockId)
  if (!block) return

  if (block['@type'] === 'dcs:Section') {
    const title = (block as { 'dcs:title'?: string })['dcs:title'] ?? ''
    writer.addSection(title, level)
    writeChildBlocks(cxt, blockId, level + 1, writer)
  } else if (block['@type'] === 'dcs:TextBlock') {
    const text = (block as { 'dcs:text'?: string })['dcs:text'] ?? ''
    const lines = text.split(NEWLINE)
    for (let i = 0; i < lines.length; i++) {
      writer.addText(lines[i] ?? '')
      if (i < lines.length - 1) writer.breakLine()
    }
    writer.addBoundarySpaceIfNeeded(text)
  } else if (block['@type'] === 'dcs:Clause') {
    writeClauseBlock(cxt, block, writer)
  }
}

function writeClauseBlock(cxt: ContractContext, clause: DcsClause, writer: PlainTextWriter): void {
  const conditions = cxt.semanticConditions
  const content = clause['dcs:content']
  const segments_raw = typeof content === 'string' ? [] : content['@list']
  const segments = parseSegmentsFromContent(segments_raw, conditions)

  for (const seg of segments) {
    if (isText(seg)) {
      writer.addText(seg.value)
    } else if (isPlaceholder(seg)) {
      const parameterValue = cxt.semanticConditionValues.find(
        // A value is keyed by its placeholder @id (conditionId), not the block.
        (item) => item.conditionId === seg.conditionId && item.parameterName === seg.parameterName,
      )?.parameterValue
      writer.addText(parameterValue == null ? DEFAULT_PLACEHOLDER_TEXT : String(parameterValue))
    } else if (isNewline(seg)) {
      writer.breakLine()
    }
  }

  const clauseText = typeof content === 'string' ? content : ''
  writer.addBoundarySpaceIfNeeded(clauseText)
}

function writeChildBlocks(cxt: ContractContext, parentBlockId: string, level: number, writer: PlainTextWriter): void {
  const node = cxt.layoutMap.get(parentBlockId)
  const childIds = node ? node['dcs:children']['@list'].map((r) => r['@id']) : []
  for (const childId of childIds) {
    writeBlockAsPlainText(cxt, childId, level, writer)
  }
}

export function useContractPlainTextConverter() {
  function convertContractToPlainTextBlocks(contractData: ContractPlainTextInput): ContractPlainTextBlock[] {
    if (!isDcsDocumentData(contractData)) return []

    const cd = contractData as import('@/models/dcs-jsonld').DcsContractData
    const blocks = cd['dcs:documentStructure']['dcs:blocks']['@list']
    const layout = cd['dcs:documentStructure']['dcs:layout']['@list']
    const conditions = getSemanticConditionsFromTemplateData(cd)
    const conditionValues = fromDocumentSemanticValues(collectDeclaredRequirements(cd))

    const context = createContractContext(blocks, layout, conditions, conditionValues)
    if (context.rootChildIds.length === 0) return []

    const writer = createPlainTextWriter()
    for (const childId of context.rootChildIds) {
      writeBlockAsPlainText(context, childId, 1, writer)
    }

    return writer.toBlocks()
  }

  return { convertContractToPlainTextBlocks }
}
