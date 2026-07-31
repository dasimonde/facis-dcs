import type { DcsBlock } from '@/models/dcs-jsonld'

/**
 * One block row in the editor list:
 * flattened outline item + block data + toolbar capabilities.
 */
export interface EnrichedBlockItem {
  /** Full JSON-LD @id IRI. */
  blockId: string
  block?: DcsBlock
  siblingIndex: number
  siblingCount: number
  /** Full JSON-LD @id IRI. */
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
}
