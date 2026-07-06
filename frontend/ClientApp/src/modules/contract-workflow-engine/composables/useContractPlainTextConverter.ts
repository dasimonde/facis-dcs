import type { SemanticConditionValue } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { SemanticCondition } from '@/modules/template-repository/models/contract-template'
import type { DcsBlock, DcsClause, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'
import { isDcsMergedApprovedTemplate } from '@template-repository/store/dcsDraftStore'
import {
  getBlocksFromTemplateData,
  getLayoutFromTemplateData,
  getSemanticConditionsFromTemplateData,
} from '@template-repository/store/dcsDraftStore'
import {
  getOwnerBlockIdFromMergedBlockId,
  isMergedBlockId,
  isSameTemplateDataRef,
} from '@template-repository/utils/template-data-ref'
import {
  parseSegmentsFromContent,
  isText,
  isPlaceholder,
  isNewline,
} from '@template-repository/composables/useClauseTextChips'
import { isDcsDocumentData } from '@/models/dcs-jsonld'

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
  blockMap: Map<string, DcsBlock | MergedApprovedTemplateBlock>
  layoutMap: Map<string, DcsLayoutNode>
  rootChildIds: string[]
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

function createContractContext(
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[],
  layout: DcsLayoutNode[],
  semanticConditions: SemanticCondition[],
  semanticConditionValues: SemanticConditionValue[],
  subTemplateSnapshots: SubTemplateSnapshot[],
): ContractContext {
  const root = layout.find((n) => n['dcs:isRoot'])
  const rootChildIds = root ? root['dcs:children']['@list'].map((r) => r['@id']) : []
  return {
    blockMap: new Map(blocks.map((b) => [b['@id'], b])),
    layoutMap: new Map(layout.map((n) => [n['@id'], n])),
    rootChildIds,
    semanticConditions,
    semanticConditionValues,
    subTemplateSnapshots,
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
  } else if (block['@type'] === 'dcs:ApprovedTemplate') {
    writeApprovedTemplateBlock(cxt, block, level, writer)
  } else if (isDcsMergedApprovedTemplate(block)) {
    writeChildBlocks(cxt, blockId, level, writer)
  }
}

function writeClauseBlock(cxt: ContractContext, clause: DcsClause, writer: PlainTextWriter): void {
  const conditions = getConditionsForBlock(clause['@id'], cxt)
  const content = clause['dcs:content']
  const segments_raw = typeof content === 'string' ? [] : content['@list']
  const segments = parseSegmentsFromContent(segments_raw, conditions)

  for (const seg of segments) {
    if (isText(seg)) {
      writer.addText(seg.value)
    } else if (isPlaceholder(seg)) {
      const parameterValue = cxt.semanticConditionValues.find(
        (item) =>
          item.blockId === clause['@id'] &&
          item.conditionId === seg.conditionId &&
          item.parameterName === seg.parameterName,
      )?.parameterValue
      writer.addText(parameterValue == null ? DEFAULT_PLACEHOLDER_TEXT : String(parameterValue))
    } else if (isNewline(seg)) {
      writer.breakLine()
    }
  }

  const clauseText = typeof content === 'string' ? content : ''
  writer.addBoundarySpaceIfNeeded(clauseText)
}

function writeApprovedTemplateBlock(
  cxt: ContractContext,
  block: import('@/models/dcs-jsonld').DcsApprovedTemplate,
  level: number,
  writer: PlainTextWriter,
): void {
  writer.breakLineIfOpen()
  const snapshot = cxt.subTemplateSnapshots.find((item) =>
    isSameTemplateDataRef(
      { templateId: item.did, version: item.version, document_number: item.document_number },
      {
        templateId: block['dcs:templateDid'],
        version: block['dcs:version'],
        document_number: block['dcs:documentNumber'],
      },
    ),
  )

  if (snapshot?.template_data) {
    const subBlocks = getBlocksFromTemplateData(snapshot.template_data)
    const subLayout = getLayoutFromTemplateData(snapshot.template_data)
    const subConditions = getSemanticConditionsFromTemplateData(snapshot.template_data)
    const snapshotContext = createContractContext(
      subBlocks,
      subLayout,
      subConditions,
      cxt.semanticConditionValues,
      cxt.subTemplateSnapshots,
    )
    for (const childId of snapshotContext.rootChildIds) {
      writeBlockAsPlainText(snapshotContext, childId, level, writer)
    }
  }

  writeChildBlocks(cxt, block['@id'], level, writer)
}

function writeChildBlocks(cxt: ContractContext, parentBlockId: string, level: number, writer: PlainTextWriter): void {
  const node = cxt.layoutMap.get(parentBlockId)
  const childIds = node ? node['dcs:children']['@list'].map((r) => r['@id']) : []
  for (const childId of childIds) {
    writeBlockAsPlainText(cxt, childId, level, writer)
  }
}

function getConditionsForBlock(blockId: string, cxt: ContractContext): SemanticCondition[] {
  if (!isMergedBlockId(blockId)) return cxt.semanticConditions
  const ownerBlockId = getOwnerBlockIdFromMergedBlockId(blockId)
  if (!ownerBlockId) return cxt.semanticConditions
  const ownerBlock = cxt.blockMap.get(ownerBlockId)
  if (!ownerBlock || !isDcsMergedApprovedTemplate(ownerBlock)) return cxt.semanticConditions
  const snapshot = cxt.subTemplateSnapshots.find((item) =>
    isSameTemplateDataRef(
      { templateId: item.did, version: item.version, document_number: item.document_number },
      {
        templateId: ownerBlock['dcs:templateDid'],
        version: ownerBlock['dcs:version'],
        document_number: ownerBlock['dcs:documentNumber'],
      },
    ),
  )
  return snapshot?.template_data
    ? getSemanticConditionsFromTemplateData(snapshot.template_data)
    : cxt.semanticConditions
}

export function useContractPlainTextConverter() {
  function convertContractToPlainTextBlocks(contractData: ContractPlainTextInput): ContractPlainTextBlock[] {
    if (!isDcsDocumentData(contractData)) return []

    const cd = contractData as import('@/models/dcs-jsonld').DcsContractData
    const blocks = cd['dcs:documentStructure']['dcs:blocks']['@list'] as (DcsBlock | MergedApprovedTemplateBlock)[]
    const layout = cd['dcs:documentStructure']['dcs:layout']
    const conditions = getSemanticConditionsFromTemplateData(cd)
    const conditionValues = cd.semanticConditionValues ?? []
    const subTemplateSnapshots: SubTemplateSnapshot[] = (cd['dcs:metadata']?.['dcs:subTemplates'] ?? []).map((s) => ({
      did: s['@id'],
      version: s['dcs:version'],
      document_number: s['dcs:documentNumber'],
      name: s['dcs:name'],
      description: s['dcs:description'],
      template_data: s['dcs:template'],
    }))

    const context = createContractContext(blocks, layout, conditions, conditionValues, subTemplateSnapshots)
    if (context.rootChildIds.length === 0) return []

    const writer = createPlainTextWriter()
    for (const childId of context.rootChildIds) {
      writeBlockAsPlainText(context, childId, 1, writer)
    }

    return writer.toBlocks()
  }

  return { convertContractToPlainTextBlocks }
}
