export const MERGED_BLOCK_ID_SEPARATOR = '::'

export interface TemplateDataRef {
  templateId: string
  version: number
  document_number?: string
}

export function isSameTemplateDataRef(a: TemplateDataRef, b: TemplateDataRef): boolean {
  return (
    a.templateId === b.templateId && a.version === b.version && (a.document_number ?? '') === (b.document_number ?? '')
  )
}

export function buildMergedChildBlockId(ownerBlockId: string, childBlockId: string): string {
  return `${ownerBlockId}${MERGED_BLOCK_ID_SEPARATOR}${childBlockId}`
}

export function isMergedBlockId(blockId: string): boolean {
  return blockId.includes(MERGED_BLOCK_ID_SEPARATOR)
}

export function getOwnerBlockIdFromMergedBlockId(mergedBlockId: string): string | undefined {
  if (!isMergedBlockId(mergedBlockId)) return undefined
  return mergedBlockId.split(MERGED_BLOCK_ID_SEPARATOR)[0]
}
