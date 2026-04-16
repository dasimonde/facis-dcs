import { defineStore } from 'pinia'
import type { TemplateDraftState, AddBlockPayload, AddBlockOptions } from "@template-repository/models/template-draft-store"
import type { DocumentOutline, DocumentOutlineBlock, DocumentBlock, TemplateTypeValue, SemanticCondition, MetaData } from "@template-repository/models/contract-templace"
import { DocumentBlockType, TemplateType, isClauseBlock, isSectionBlock, isApprovedTemplateBlock } from "@template-repository/models/contract-templace"
import type { ContractTemplate, SubTemplateSnapshot } from '@/models/contract-template'
import type { ContractTemplateCreateRequest, ContractTemplateUpdateRequest } from '@/models/requests/template-request'

const storeId = "templateDraft"
const defaultState: Readonly<TemplateDraftState> = {
  did: null,
  name: '',
  description: '',
  documentOutline: [],
  documentBlocks: [],
  semanticConditions: [],
  customMetaData: [],
  subTemplateSnapshots: [],
  templateType: TemplateType.subContract,
  state: null,
  document_number: null,
  version: null,
  updated_at: null,
  created_by: '',
}

export const useTemplateDraftStore = defineStore(storeId, {
  state: (): TemplateDraftState => getInitialState(),
  getters: {
    hasTemplateId(): boolean { return !!this.did },
    /** Set of all block IDs that appear in the document outline tree (root + all descendants). */
    blockIdsInOutline(): Set<string> {
      return collectBlockIdsInOutline(this.documentOutline)
    },
    /** Returns the data to create a contract template based on the current draft state. */
    templateCreateRequestData(): ContractTemplateCreateRequest {
      return {
        name: this.name,
        description: this.description,
        template_type: this.templateType,
        template_data: {
          documentOutline: this.documentOutline,
          documentBlocks: this.documentBlocks,
          semanticConditions: this.semanticConditions,
          customMetaData: this.customMetaData,
          subTemplateSnapshots: this.subTemplateSnapshots,
        }
      }
    },
    templateUpdateRequestData(): ContractTemplateUpdateRequest | null {
      if (!this.did || !this.updated_at) return null
      return {
        did: this.did,
        updated_at: this.updated_at,
        name: this.name,
        description: this.description,
        template_data: {
          documentOutline: this.documentOutline,
          documentBlocks: this.documentBlocks,
          semanticConditions: this.semanticConditions,
          customMetaData: this.customMetaData,
          subTemplateSnapshots: this.subTemplateSnapshots,
        },
      }
    }
  },
  actions: {
    // Block operations: add, delete, update, move
    /**
     * Adds a new block under the given parent at the given index.
     * 
     * subContract: cannot add APPROVED_TEMPLATE. frameContract: can only add APPROVED_TEMPLATE.
     * 
     * @param parentBlockId - blockId of the outline node (parent) under which to insert
     * @param insertIndex - index in the parent's children array (0 = first)
     * @param options.addToOutline - when false, new block is only added to documentBlocks (default true)
     * @returns The new or inserted block's blockId.
     */
    addBlock(parentBlockId: string, insertIndex: number, payload: AddBlockPayload, options?: AddBlockOptions): string {
      if (this.templateType === TemplateType.subContract && payload.blockType === DocumentBlockType.ApprovedTemplate) {
        throw new Error('subContract template cannot add APPROVED_TEMPLATE blocks')
      }
      if (this.templateType === TemplateType.frameContract && payload.blockType !== DocumentBlockType.ApprovedTemplate) {
        throw new Error('frameContract template can only add APPROVED_TEMPLATE blocks')
      }
      return addBlock(this.documentOutline, this.documentBlocks, parentBlockId, insertIndex, payload, options)
    },
    /** Removes the block and all its descendants from documentOutline and documentBlocks. */
    deleteBlock(blockId: string): void {
      deleteBlock(this.documentOutline, this.documentBlocks, blockId)
    },
    /** Updates block fields. */
    updateBlock(blockId: string, payload: { title?: string; text?: string; conditionIds?: string[] }): void {
      const block = this.documentBlocks.find((b) => b.blockId === blockId)
      if (!block) return
      if (payload.title !== undefined) block.title = payload.title
      if (payload.text !== undefined) block.text = payload.text
      if (isClauseBlock(block)) block.conditionIds = payload.conditionIds ?? []

    },
    /**
     * Moves a block to a new position under the same or another parent.
     * @param blockId - block to move
     * @param parentBlockId - outline node (parent) under which to place the block
     * @param insertIndex - index in the parent's children array (0 = first)
     */
    moveBlock(blockId: string, parentBlockId: string, insertIndex: number): void {
      moveBlock(this.documentOutline, blockId, parentBlockId, insertIndex)
    },
    // Semantic Rules operations: add, delete
    addSemanticCondition(payload: Omit<SemanticCondition, 'conditionId'>): void {
      this.semanticConditions.push({
        ...payload,
        conditionId: crypto.randomUUID(),
      })
    },
    deleteSemanticCondition(conditionId: string): void {
      const placeholderRegex = placeholderRegexForCondition(conditionId)
      for (const block of this.documentBlocks) {
        if (!isClauseBlock(block)) continue
        const hadCondition = block.conditionIds.includes(conditionId)
        block.conditionIds = block.conditionIds.filter((id) => id !== conditionId)
        if (hadCondition) block.text = block.text.replace(placeholderRegex, '')
      }
      this.semanticConditions = this.semanticConditions.filter((c) => c.conditionId !== conditionId)
    },
    // Clauses operations: add, delete, update
    /** Adds a clause block to documentBlocks only */
    addClause(payload: { title?: string; text: string; conditionIds: string[] }): string {
      const blockId = crypto.randomUUID()
      const block = createBlockFromPayload(blockId, {
        blockType: DocumentBlockType.Clause,
        text: payload.text,
        title: payload.title,
        conditionIds: payload.conditionIds,
      })
      this.documentBlocks.push(block)
      return blockId
    },
    /** Removes the clause from documentBlocks and documentOutline. */
    deleteClause(blockId: string): void {
      this.documentBlocks = this.documentBlocks.filter((b) => b.blockId !== blockId)
      const parent = this.documentOutline.find((b) => b.children.includes(blockId))
      if (parent) {
        parent.children = parent.children.filter((id) => id !== blockId)
      }
    },
    updateClause(blockId: string, payload: { title?: string; text?: string; conditionIds?: string[] }): void {
      this.updateBlock(blockId, payload)
    },
    // MetaData operations: add, delete, update
    addMetaData(payload: MetaData): boolean {
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m) => m.name.trim().toLowerCase() === lower)
      if (hasDuplicate) return false
      this.customMetaData.push({ name, value })
      return true
    },
    deleteMetaData(index: number): void {
      if (index < 0 || index >= this.customMetaData.length) return
      this.customMetaData.splice(index, 1)
    },
    updateMetaData(index: number, payload: MetaData): boolean {
      if (index < 0 || index >= this.customMetaData.length) return false
      const name = payload.name.trim()
      const value = payload.value
      if (!name) return false
      const lower = name.toLowerCase()
      const hasDuplicate = this.customMetaData.some((m, idx) => {
        if (idx === index) return false
        return m.name.trim().toLowerCase() === lower
      })
      if (hasDuplicate) return false
      this.customMetaData[index] = { name, value }
      return true
    },
    // Basic info operations
    updateTemplateType(templateType: TemplateTypeValue): void {
      if (this.did !== null && this.did !== undefined) {
        throw new Error('Cannot change template type after template is created')
      }
      this.templateType = templateType
      /**  TBD: after changing template type, the blocks that are not allowed in the new 
       * template type should be removed. For example, if changing from frameContract 
       * to subContract, the APPROVED_TEMPLATE blocks should be removed. */
    },
    updateName(name: string): void {
      this.name = name
    },
    updateDescription(description: string): void {
      this.description = description
    },
    addSubTemplateSnapshot(template: ContractTemplate): void {
      const snapshot:SubTemplateSnapshot = {
        did: template.did,
        version: template.version,
        document_number: template.document_number,
        name: template.name,
        description: template.description,
        template_data: template.template_data,
      }
      this.subTemplateSnapshots = [
        ... this.subTemplateSnapshots.filter((item) => !isSameTemplate(item, snapshot)),
        snapshot
      ]
    },
    removeSubTemplateSnapshot(template: { did: string, version?: number, document_number?: string }): void {
      this.subTemplateSnapshots = this.subTemplateSnapshots.filter((item) => !isSameTemplate(item, template))
    },
    reset(overrides?: Partial<TemplateDraftState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    }
  }
})

/** Creates a document outline item */
function createOutlineItem(overrides?: Partial<Pick<DocumentOutlineBlock, 'blockId' | 'isRoot' | 'children'>>): DocumentOutlineBlock {
  return {
    blockId: overrides?.blockId ?? crypto.randomUUID(),
    isRoot: overrides?.isRoot ?? false,
    children: overrides?.children ?? [],
  }
}

/** Returns the set of all block IDs in the outline tree (root + all descendants). */
function collectBlockIdsInOutline(outline: DocumentOutline): Set<string> {
  const blockIds = new Set<string>()
  const blockMap = new Map(outline.map((b) => [b.blockId, b]))
  function visit(blockId: string) {
    if (blockIds.has(blockId)) return
    blockIds.add(blockId)
    const block = blockMap.get(blockId)
    if (block) block.children.forEach(visit)
  }
  const root = outline.find((b) => b.isRoot)
  if (root) visit(root.blockId)
  return blockIds
}

function addBlock(
  outline: DocumentOutlineBlock[],
  blocks: DocumentBlock[],
  parentBlockId: string,
  insertIndex: number,
  payload: AddBlockPayload,
  options?: AddBlockOptions
): string {
  const addToOutline = options?.addToOutline !== false

  // Clause block is created from ClausesEditor and only added to documentOutline in BuilderEditor.
  if (payload.clauseBlockId) {
    const clauseBlockId = payload.clauseBlockId
    const block = blocks.find((b) => b.blockId === clauseBlockId)
    if (!block || !isClauseBlock(block)) {
      throw new Error(`addBlock: clause block not found: ${clauseBlockId}`)
    }
    const inOutline = collectBlockIdsInOutline(outline)
    if (inOutline.has(clauseBlockId)) {
      return clauseBlockId
    }
    const parent = outline.find((b) => b.blockId === parentBlockId)
    if (!parent) {
      throw new Error(`addBlock: parent not found: ${parentBlockId}`)
    }
    parent.children.splice(insertIndex, 0, clauseBlockId)
    return clauseBlockId
  }

  const blockId = crypto.randomUUID()
  const block = createBlockFromPayload(blockId, payload)

  if (isClauseBlock(block) && !addToOutline) {
    blocks.push(block)
    return blockId
  }

  const parent = outline.find((b) => b.blockId === parentBlockId)
  if (!parent) {
    throw new Error(`addBlock: parent not found: ${parentBlockId}`)
  }
  parent.children.splice(insertIndex, 0, blockId)
  if (isSectionBlock(block) || isApprovedTemplateBlock(block)) {
    outline.push(createOutlineItem({ blockId, isRoot: false, children: [] }))
  }
  blocks.push(block)
  return blockId
}

/**
 * Moves a block within the outline (same parent or different parent). Mutates documentOutline only.
 */
function moveBlock(
  outline: DocumentOutlineBlock[],
  blockId: string,
  parentBlockId: string,
  insertIndex: number
): void {
  const oldParent = outline.find((b) => b.children.includes(blockId))
  const newParent = outline.find((b) => b.blockId === parentBlockId)
  if (!oldParent || !newParent) return

  // same parent
  if (oldParent.blockId === newParent.blockId) {
    const siblings = oldParent.children.filter((id) => id !== blockId)
    const idx = Math.min(insertIndex, siblings.length)
    siblings.splice(idx, 0, blockId)
    oldParent.children = siblings
    return
  }
  // different parent
  oldParent.children = oldParent.children.filter((id) => id !== blockId)
  const newSiblings = [...newParent.children]
  const idx = Math.min(insertIndex, newSiblings.length)
  newSiblings.splice(idx, 0, blockId)
  newParent.children = newSiblings
}

/**
 * Removes the block and all its descendants from outline and blocks. Mutates both arrays.
 */
function deleteBlock(
  outline: DocumentOutlineBlock[],
  blocks: DocumentBlock[],
  blockId: string
): void {
  const block = blocks.find((b) => b.blockId === blockId)
  const parent = outline.find((b) => b.children.includes(blockId))
  if (!parent) {
    return
  }
  // Clause: only remove from outline so the clause can be re-used from Add block modal.
  if (block && isClauseBlock(block)) {
    parent.children = parent.children.filter((id) => id !== blockId)
    return
  }
  const outlineByBlockId = new Map(outline.map((b) => [b.blockId, b]))
  const toRemove = collectDescendantBlockIds(blockId, outlineByBlockId)
  parent.children = parent.children.filter((id) => id !== blockId)
  const outlineToKeep = outline.filter((b) => b.isRoot === true || !toRemove.has(b.blockId))
  outline.length = 0
  outline.push(...outlineToKeep)
  const blocksToKeep = blocks.filter((b) => !toRemove.has(b.blockId))
  blocks.length = 0
  blocks.push(...blocksToKeep)
}

function createBlockFromPayload(blockId: string, payload: AddBlockPayload): DocumentBlock {
  const text = payload.text ?? ''
  switch (payload.blockType) {
    case DocumentBlockType.Section:
      return { blockId, type: DocumentBlockType.Section, text }
    case DocumentBlockType.Text:
      return { blockId, type: DocumentBlockType.Text, text }
    case DocumentBlockType.Clause:
      return { blockId, type: DocumentBlockType.Clause, text, title: payload.title, conditionIds: payload.conditionIds ?? [] }
    case DocumentBlockType.ApprovedTemplate:
      return {
        blockId,
        type: DocumentBlockType.ApprovedTemplate,
        text,
        templateId: payload.templateId ?? '',
        version: payload.version ?? 1,
        document_number: payload.document_number ?? '',
      }
    default:
      throw new Error(`Unknown blockType: ${payload.blockType}`)
  }
}

/** Returns a Set of blockId and all descendant block ids in the outline. */
function collectDescendantBlockIds(
  blockId: string,
  outlineByBlockId: Map<string, DocumentOutlineBlock>
): Set<string> {
  const set = new Set<string>([blockId])
  const node = outlineByBlockId.get(blockId)
  const childIds = node?.children ?? []
  childIds.forEach((id) => collectDescendantBlockIds(id, outlineByBlockId).forEach((x) => set.add(x)))
  return set
}

/** Regex to match {{conditionId.parameterName}}. */
function placeholderRegexForCondition(conditionId: string): RegExp {
  return new RegExp(`\\{\\{${conditionId}\\.([^}]*)\\}\\}`, 'g')
}

/** Returns a copy of defaultState so store state does not share 
 *  refs with defaultState; mutations in the store do not pollute 
 *  defaultState and $reset() restores correctly.
 **/
function getInitialState(): TemplateDraftState {
  return {
    ...defaultState,
    // Root is not created by the user
    documentOutline: [createOutlineItem({ isRoot: true, children: [] })],
    documentBlocks: [...defaultState.documentBlocks],
    semanticConditions: [...defaultState.semanticConditions],
    customMetaData: [...defaultState.customMetaData],
    subTemplateSnapshots: [...defaultState.subTemplateSnapshots],
  }
}

function isSameTemplate(
  t1: { did: string, version?: number, document_number?: string },
  t2: { did: string, version?: number, document_number?: string }
): boolean {
  return t1.did === t2.did && t1.version === t2.version && t1.document_number === t2.document_number
}