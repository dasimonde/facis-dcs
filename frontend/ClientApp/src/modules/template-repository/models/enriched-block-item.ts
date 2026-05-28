import type { DocumentBlock, MergedApprovedTemplateBlock } from './contract-templace'

/**
 * One block row in the editor list:
 * flattened outline item + block data + toolbar capabilities.
 */
export interface EnrichedBlockItem {
  blockId: string
  block?: DocumentBlock
  siblingIndex: number
  siblingCount: number
  parentBlockId: string
  depthLevel: number
  prevSiblingBlockId?: string
  nextSiblingBlockId?: string
  canOutdent: boolean
  canIndent: boolean
  outdentGrandparentBlockId: string
  outdentInsertIndex: number
  indentParentBlockId: string
  indentInsertIndex: number
  /**
   * Some approved blocks are merged into the main document for editing
   * without conflicting with the original approved template.
   */
  mergedApprovedBlock?: MergedApprovedTemplateBlock
}
