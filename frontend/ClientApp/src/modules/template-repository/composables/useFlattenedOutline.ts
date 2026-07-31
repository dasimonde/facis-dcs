import { computed, type MaybeRef, unref } from 'vue'
import type { DcsLayoutNode } from '@/models/dcs-jsonld'

export interface FlattenedOutlineItem {
  /** Full JSON-LD @id IRI of this block. */
  blockId: string
  /** Full JSON-LD @id IRI of the parent node. */
  parentBlockId: string
  /** 0-based index of this block among its siblings (in the parent's children array). */
  siblingIndex: number
  /** 1-based section numbers from root to this block (e.g. [1, 2] for "Section 1.2"). */
  sectionNumberPath: number[]
  /** 0-based tree depth; 0 = direct child of the document root. */
  depthLevel: number
}

function layoutNodeChildIds(node: DcsLayoutNode): string[] {
  return node['dcs:children']['@list'].map((ref) => ref['@id'])
}

function flattenLayout(layout: DcsLayoutNode[]): FlattenedOutlineItem[] {
  const nodeById = new Map<string, DcsLayoutNode>(layout.map((n) => [n['@id'], n]))
  const root = layout.find((n) => n['dcs:isRoot'])
  const result: FlattenedOutlineItem[] = []

  function collect(
    iri: string,
    parentIri: string,
    siblingIndex: number,
    depthLevel: number,
    sectionNumberPath: number[],
  ) {
    result.push({ blockId: iri, parentBlockId: parentIri, siblingIndex, sectionNumberPath, depthLevel })
    const node = nodeById.get(iri)
    const childIris = node ? layoutNodeChildIds(node) : []
    childIris.forEach((id, i) => collect(id, iri, i, depthLevel + 1, [...sectionNumberPath, i + 1]))
  }

  if (root) {
    layoutNodeChildIds(root).forEach((id, i) => collect(id, root['@id'], i, 0, [i + 1]))
  }
  return result
}

/** Composable: returns a computed depth-first flat list from a JSON-LD layout (ref or raw). */
export function useFlattenedOutline(layout: MaybeRef<DcsLayoutNode[]>) {
  return computed(() => flattenLayout(unref(layout)))
}
