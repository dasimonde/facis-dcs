import { computed, unref, type MaybeRef } from 'vue'
import type { DocumentOutline, DocumentOutlineBlock } from '@template-repository/models/contract-templace'

export interface FlattenedOutlineItem {
  blockId: string
  parentBlockId: string
  /** 0-based index of this block among its siblings (in the parent's children array). */
  siblingIndex: number
  /** 1-based section numbers from root to this block (e.g. [1, 2] for "Section 1.2"). */
  sectionNumberPath: number[]
  /** 0-based tree depth; 0 = direct child of the document root. */
  depthLevel: number
}

/** Converts the document outline tree into a depth-first flat list of items with positional metadata. */
function flattenOutline(outline: DocumentOutline): FlattenedOutlineItem[] {
  const outlineByBlockId = new Map<string, DocumentOutlineBlock>(outline.map((b) => [b.blockId, b]))
  const root = outline.find((b) => b.isRoot)
  const rootChildIds = root?.children ?? []
  const result: FlattenedOutlineItem[] = []

  function collect(
    blockId: string,
    parentBlockId: string,
    siblingIndex: number,
    depthLevel: number,
    sectionNumberPath: number[],
  ) {
    result.push({ blockId, parentBlockId, siblingIndex, sectionNumberPath, depthLevel })
    const outlineBlock = outlineByBlockId.get(blockId)
    const childIds = outlineBlock?.children ?? []
    childIds.forEach((id, i) => collect(id, blockId, i, depthLevel + 1, [...sectionNumberPath, i + 1]))
  }

  if (root) {
    rootChildIds.forEach((id, i) => collect(id, root.blockId, i, 0, [i + 1]))
  }
  return result
}

/** Composable: returns a computed depth-first flat list from a document outline (ref or raw). */
export function useFlattenedOutline(outline: MaybeRef<DocumentOutline>) {
  return computed(() => flattenOutline(unref(outline)))
}
