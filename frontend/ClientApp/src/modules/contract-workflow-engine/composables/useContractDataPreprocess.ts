import type { ContractData } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type {
  ApprovedTemplateBlock,
  DocumentBlock,
  DocumentOutlineBlock,
} from '@template-repository/models/contract-templace'
import {
  DocumentBlockType,
  isApprovedTemplateBlock,
  isMergedApprovedTemplateBlock,
} from '@template-repository/models/contract-templace'
import { buildMergedChildBlockId, isSameTemplateDataRef } from '@template-repository/utils/template-data-ref'
import { TEMPLATE_DATA_VERSIONS, type TemplateDataVersion } from '@template-repository/models/template-draft-store'

const CURRENT_TEMPLATE_DATA_VERSION: TemplateDataVersion = TEMPLATE_DATA_VERSIONS[0]

/**
 * Preprocesses contract data for use in the contract workflow engine:
 * - Convert the approved template blocks into main template blocks
 * - Support editing multi-contract templates, even when they are the same template but with different versions
 * @returns contract data
 */
export function useContractDataPreprocess() {
  function preprocessContractData(cd: ContractData): ContractData {
    const contractData: ContractData = {
      documentOutline: deepClone(cd.documentOutline ?? []),
      documentBlocks: deepClone(cd.documentBlocks ?? []),
      semanticConditions: deepClone(cd.semanticConditions ?? []),
      subTemplateSnapshots: deepClone(cd.subTemplateSnapshots ?? []),
      semanticConditionValues: deepClone(cd.semanticConditionValues ?? []),
      templateDataVersion: normalizeTemplateDataVersion(cd.templateDataVersion),
    }

    const approvedBlocks = contractData.documentBlocks.filter(isApprovedTemplateBlock)
    if (approvedBlocks.length === 0) return contractData
    for (const approvedBlock of approvedBlocks) {
      const subTemplateData = findSnapshotByApprovedBlock(
        contractData.subTemplateSnapshots,
        approvedBlock,
      )?.template_data
      const approvedOutlineNode = contractData.documentOutline.find((b) => b.blockId === approvedBlock.blockId)
      const subRootOutlineBlock = subTemplateData?.documentOutline.find((b) => b.isRoot)
      if (!subTemplateData || !approvedOutlineNode || !subRootOutlineBlock) continue

      /**
       * APPROVED_TEMPLATE blocks may point to the same template. Use an
       * remap so block IDs are unique across injections and stable across reloads.
       */
      const blockIdMap = buildBlockIdMap(subTemplateData.documentOutline, approvedBlock.blockId)
      const mergedOutline: DocumentOutlineBlock[] = subTemplateData.documentOutline
        .filter((b) => !b.isRoot)
        .map((b) => ({
          ...b,
          blockId: getMappedBlockId(blockIdMap, b.blockId),
          children: b.children.map((childId) => getMappedBlockId(blockIdMap, childId)),
        }))
      const mergedBlocks: DocumentBlock[] = subTemplateData.documentBlocks.map((b) => ({
        ...b,
        blockId: getMappedBlockId(blockIdMap, b.blockId),
      }))
      const mergedChildIds = subRootOutlineBlock.children.map((childId) => getMappedBlockId(blockIdMap, childId))

      // Convert block type and merge blocks
      contractData.documentBlocks = contractData.documentBlocks.filter((b) => b.blockId !== approvedBlock.blockId)
      contractData.documentBlocks.push({
        ...approvedBlock,
        type: DocumentBlockType.MergedApprovedTemplate,
      })
      contractData.documentBlocks.push(...mergedBlocks)

      // Update main outline
      approvedOutlineNode.children = [...mergedChildIds, ...approvedOutlineNode.children]
      contractData.documentOutline.push(...mergedOutline)
      contractData.documentOutline = removeMergedApprovedChildrenRefs(
        contractData.documentOutline,
        contractData.documentBlocks,
      )

      ensureOutlineNodesExist(contractData.documentOutline, approvedOutlineNode.children)
    }
    return contractData
  }

  return { preprocessContractData }
}

function removeMergedApprovedChildrenRefs(
  outlineBlocks: DocumentOutlineBlock[],
  documentBlocks: DocumentBlock[],
): DocumentOutlineBlock[] {
  // MERGED_APPROVED_TEMPLATE nodes should never appear in any parent.children.
  // Replace each merged child reference with its descendants recursively.
  const mergedApprovedBlockIds = new Set(
    documentBlocks.filter((b) => isMergedApprovedTemplateBlock(b)).map((b) => b.blockId),
  )
  if (mergedApprovedBlockIds.size === 0) return outlineBlocks

  const rebuiltOutline: DocumentOutlineBlock[] = outlineBlocks.map((b) => ({
    ...b,
    children: [...b.children],
  }))
  const outlineByBlockId = new Map(rebuiltOutline.map((o) => [o.blockId, o]))
  // key: blockId, value: children
  const flattenedChildrenCache = new Map<string, string[]>()

  function resolveChildrenWithoutMerged(blockId: string, blockPath = new Set<string>()): string[] {
    const resolvedChildren: string[] = []
    const cachedChildren = flattenedChildrenCache.get(blockId)
    // console.log(blockId, cachedChildren)
    if (cachedChildren) return cachedChildren
    if (blockPath.has(blockId)) return resolvedChildren
    blockPath.add(blockId)

    const outlineNode = outlineByBlockId.get(blockId)
    if (!outlineNode) {
      blockPath.delete(blockId)
      return resolvedChildren
    }

    // Get children
    for (const childId of outlineNode.children) {
      if (!mergedApprovedBlockIds.has(childId)) {
        if (!resolvedChildren.includes(childId)) {
          resolvedChildren.push(childId)
        }
        continue
      }
      const grandChildren = resolveChildrenWithoutMerged(childId, blockPath)
      for (const childId of grandChildren) {
        if (!resolvedChildren.includes(childId)) {
          resolvedChildren.push(childId)
        }
      }
    }

    blockPath.delete(blockId)
    flattenedChildrenCache.set(blockId, resolvedChildren)
    return resolvedChildren
  }

  for (const outlineNode of rebuiltOutline) {
    outlineNode.children = resolveChildrenWithoutMerged(outlineNode.blockId)
  }

  // Keep merged nodes in outline list
  for (const mergedId of mergedApprovedBlockIds) {
    const mergedNode = outlineByBlockId.get(mergedId)
    if (mergedNode) mergedNode.children = []
  }
  return rebuiltOutline
}

function normalizeTemplateDataVersion(version: unknown): TemplateDataVersion {
  if (!Number.isInteger(version)) return CURRENT_TEMPLATE_DATA_VERSION
  if (TEMPLATE_DATA_VERSIONS.includes(version as TemplateDataVersion)) return version as TemplateDataVersion
  return CURRENT_TEMPLATE_DATA_VERSION
}

function findSnapshotByApprovedBlock(
  subTemplates: SubTemplateSnapshot[],
  approvedBlock: ApprovedTemplateBlock,
): SubTemplateSnapshot | undefined {
  const exact = subTemplates.find((snapshot) =>
    isSameTemplateDataRef(
      {
        templateId: snapshot.did,
        version: snapshot.version,
        document_number: snapshot.document_number,
      },
      {
        templateId: approvedBlock.templateId,
        version: approvedBlock.version,
        document_number: approvedBlock.document_number,
      },
    ),
  )
  if (exact) return exact
  return subTemplates.find((snapshot) => snapshot.did === approvedBlock.templateId)
}

/**
 * Builds a map of old block IDs to new block IDs.
 * The root block ID is excluded because it won't be merged into main template.
 * @param outline DocumentOutlineBlock[]
 * @returns block ID map
 */
function buildBlockIdMap(outline: DocumentOutlineBlock[], approvedBlockId: string): Map<string, string> {
  const map = new Map<string, string>()
  for (const outlineBlock of outline) {
    if (!outlineBlock.isRoot && !map.has(outlineBlock.blockId)) {
      map.set(outlineBlock.blockId, buildMergedChildBlockId(approvedBlockId, outlineBlock.blockId))
    }
    for (const childId of outlineBlock.children) {
      if (!map.has(childId)) map.set(childId, buildMergedChildBlockId(approvedBlockId, childId))
    }
  }
  return map
}

function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function ensureOutlineNodesExist(outline: DocumentOutlineBlock[], blockIds: string[]) {
  const existing = new Set(outline.map((block) => block.blockId))
  for (const blockId of blockIds) {
    if (existing.has(blockId)) continue
    outline.push({
      isRoot: false,
      blockId,
      children: [],
    })
    existing.add(blockId)
  }
}

function getMappedBlockId(blockIdMap: Map<string, string>, blockId: string): string {
  return blockIdMap.get(blockId) ?? blockId
}
