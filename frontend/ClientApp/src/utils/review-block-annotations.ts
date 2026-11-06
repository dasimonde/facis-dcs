import type { DcsBlock, DcsClause, DcsContentSegment, DcsLayoutNode } from '@/models/dcs-jsonld'
import type { ContractBlockAnnotation } from '@/models/requests/contract-request'

export interface AnnotatableBlock {
  blockId: string
  /** Short title or heading when available (sections / named clauses). */
  label: string
  type: string
  /** Body text excerpt matching what appears in Contract Content (clauses / text). */
  preview: string
}

/** Draft annotation map: block @id → comment text (frontend-only until submit). */
export type ReviewAnnotationDraft = Record<string, string>

const PREVIEW_MAX = 80

function truncatePreview(text: string, max = PREVIEW_MAX): string {
  const compact = text.replace(/\s+/g, ' ').trim()
  if (!compact) return ''
  return compact.length > max ? `${compact.slice(0, max)}…` : compact
}

/** Flatten clause segments to plain text; field refs become […]. */
export function clauseContentPreview(clause: DcsClause): string {
  const content = clause['dcs:content']
  if (typeof content === 'string') return truncatePreview(content)
  const list: DcsContentSegment[] = content?.['@list'] ?? []
  const parts: string[] = []
  for (const seg of list) {
    if (typeof seg === 'string') parts.push(seg)
    else if (seg && typeof seg === 'object') parts.push('[…]')
  }
  return truncatePreview(parts.join(''))
}

/** Depth-first block @ids in the same order as Contract Content / TemplatePreview. */
export function orderedBlockIdsFromLayout(layout: DcsLayoutNode[]): string[] {
  const nodeById = new Map(layout.map((n) => [n['@id'], n]))
  const root = layout.find((n) => n['dcs:isRoot'])
  if (!root) return []
  const out: string[] = []
  function walk(iri: string) {
    out.push(iri)
    const children = nodeById.get(iri)?.['dcs:children']['@list'] ?? []
    for (const ref of children) walk(ref['@id'])
  }
  for (const ref of root['dcs:children']['@list']) walk(ref['@id'])
  return out
}

function describeBlock(block: DcsBlock): AnnotatableBlock | null {
  const blockId = typeof block['@id'] === 'string' ? block['@id'].trim() : ''
  if (!blockId) return null
  const type =
    typeof block['@type'] === 'string'
      ? block['@type']
      : Array.isArray(block['@type'])
        ? String(block['@type'][0] ?? '')
        : ''
  const local = type.includes(':') ? type.slice(type.lastIndexOf(':') + 1) : type
  let label = local.length > 0 ? local : 'Block'
  let preview = ''
  if (local === 'Section') {
    const title = (block as { 'dcs:title'?: string })['dcs:title']?.trim()
    label = title !== undefined && title.length > 0 ? title : 'Section'
  } else if (local === 'Clause') {
    const clause = block as DcsClause
    const title = clause['dcs:title']?.trim()
    preview = clauseContentPreview(clause)
    // Content preview is what reviewers see in Contract Content.
    if (preview.length > 0) label = preview
    else if (title !== undefined && title.length > 0) label = title
    else label = 'Clause'
  } else if (local === 'TextBlock') {
    const text = (block as { 'dcs:text'?: string })['dcs:text']?.trim() ?? ''
    preview = truncatePreview(text)
    label = preview.length > 0 ? preview : 'Text'
  } else {
    return null
  }
  return {
    blockId,
    label,
    type: local.length > 0 ? local : 'Block',
    preview,
  }
}

function orderAnnotatableBlocks(byId: Map<string, AnnotatableBlock>, layout?: DcsLayoutNode[]): AnnotatableBlock[] {
  if (!layout?.length) return [...byId.values()]
  const ordered: AnnotatableBlock[] = []
  const seen = new Set<string>()
  for (const id of orderedBlockIdsFromLayout(layout)) {
    const item = byId.get(id)
    if (!item || seen.has(id)) continue
    ordered.push(item)
    seen.add(id)
  }
  for (const item of byId.values()) {
    if (!seen.has(item.blockId)) ordered.push(item)
  }
  return ordered
}

/** All document blocks with human-readable labels (for audit display). Never uses raw @id as label. */
export function listBlockContentLabels(blocks: DcsBlock[], layout?: DcsLayoutNode[]): AnnotatableBlock[] {
  const byId = new Map<string, AnnotatableBlock>()
  for (const block of blocks) {
    const described = describeBlock(block)
    if (described) byId.set(described.blockId, described)
  }
  return orderAnnotatableBlocks(byId, layout)
}

export function listAnnotatableBlocks(blocks: DcsBlock[], layout?: DcsLayoutNode[]): AnnotatableBlock[] {
  // Review annotations target clauses only for now (section/text edit not opened yet).
  return listBlockContentLabels(blocks, layout).filter((b) => b.type === 'Clause')
}

/** Resolve a display label for an annotated block; never returns a raw @id / URL. */
export function resolveAnnotationContentLabel(blockId: string, blocks: DcsBlock[], layout?: DcsLayoutNode[]): string {
  const found = listBlockContentLabels(blocks, layout).find((b) => b.blockId === blockId)
  if (found?.label && !looksLikeBlockId(found.label)) return found.label
  return 'Content'
}

function looksLikeBlockId(text: string): boolean {
  return text.includes('://') || text.includes('#block-')
}

export function filterAnnotatableBlocks(blocks: AnnotatableBlock[], query: string): AnnotatableBlock[] {
  const q = query.trim().toLowerCase()
  if (!q) return blocks
  return blocks.filter(
    (b) => b.label.toLowerCase().includes(q) || b.type.toLowerCase().includes(q) || b.preview.toLowerCase().includes(q),
  )
}

export function draftToAnnotations(draft: ReviewAnnotationDraft): ContractBlockAnnotation[] {
  const out: ContractBlockAnnotation[] = []
  for (const [blockId, comment] of Object.entries(draft)) {
    const trimmed = comment.trim()
    if (!blockId.trim() || !trimmed) continue
    out.push({ block_id: blockId, comment: trimmed })
  }
  return out
}

export function annotationCount(draft: ReviewAnnotationDraft): number {
  return draftToAnnotations(draft).length
}

/** Entries added in the panel but still missing comment text. */
export function incompleteAnnotationCount(draft: ReviewAnnotationDraft): number {
  let n = 0
  for (const [blockId, comment] of Object.entries(draft)) {
    if (blockId.trim() && !comment.trim()) n += 1
  }
  return n
}
