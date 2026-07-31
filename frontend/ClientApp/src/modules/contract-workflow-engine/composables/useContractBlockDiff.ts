import { diff } from 'fast-myers-diff'
import type { ContractPlainTextBlock } from './useContractPlainTextConverter'

export type DiffType = 'added' | 'removed' | 'modified'
export type TextDiffSegmentType = 'equal' | 'added' | 'removed'

export interface ContractDiffDocument {
  summary: ContractDiffSummary
  leftRows: ContractDiffRow[]
  rightRows: ContractDiffRow[]
}

export interface ContractDiffSummary {
  unchangedCount: number
  addedCount: number
  removedCount: number
  modifiedCount: number
}

export interface ContractDiffRow {
  /** format: `${left-or-right}-${lineNumber}` */
  id: string
  type: DiffType
  /** The line number of the block in the original document, starts from 1 */
  lineNumber: number
  block: ContractPlainTextBlock
  /**
   * word-level diff
   *
   * "The cost is 123."
   * => ['The cost is ', '123', '.']
   * => [{"type":"equal","text":"The cost is "},{"type":"removed","text":"123"},{"type":"added","text":"456"},{"type":"equal","text":"."}]
   */
  segments?: TextDiffSegment[]
}

export interface TextDiffSegment {
  type: TextDiffSegmentType
  text: string
}

/**
 * [leftStart, leftEnd) indicates a range to delete from leftComparable
 *
 * [rightStart, rightEnd) indicates a range from rightComparable to replace the deleted material with
 *
 * Simple deletions are indicated when rightStart === rightEnd
 *
 * Simple insertions when leftStart === leftEnd
 */
type Patch = [leftStart: number, leftEnd: number, rightStart: number, rightEnd: number]

export function useContractBlockDiff() {
  function buildContractBlockDiff(
    leftBlocks: ContractPlainTextBlock[],
    rightBlocks: ContractPlainTextBlock[],
  ): ContractDiffDocument {
    const leftRows: ContractDiffRow[] = []
    const rightRows: ContractDiffRow[] = []
    const summary: ContractDiffSummary = {
      unchangedCount: 0,
      addedCount: 0,
      removedCount: 0,
      modifiedCount: 0,
    }

    const leftComparable = leftBlocks.map(getComparableBlockText)
    const rightComparable = rightBlocks.map(getComparableBlockText)
    const patches: Patch[] = Array.from(diff(leftComparable, rightComparable))

    for (const patch of patches) {
      const isModified = updateBothSideRowsIfIsModifiedBlock(
        patch,
        leftBlocks,
        rightBlocks,
        leftRows,
        rightRows,
        summary,
      )
      if (isModified) continue
      updateLeftRowIfIsDeletedBlock(patch, leftBlocks, leftRows, summary)
      updateRightRowIfIsAddedBlock(patch, rightBlocks, rightRows, summary)
    }

    summary.unchangedCount =
      Math.max(leftBlocks.length, rightBlocks.length) -
      summary.addedCount -
      summary.removedCount -
      summary.modifiedCount

    return {
      summary,
      leftRows,
      rightRows,
    }
  }

  return { buildContractBlockDiff }
}

function getComparableBlockText(block: ContractPlainTextBlock): string {
  if (block.type === 'section') return `section:${block.level}:${block.text}`
  return `text:${block.text}`
}

const WORD_RE = String.raw`\p{L}[\p{L}\p{M}\p{N}_-]*`
const NUMBER_RE = String.raw`\p{N}+`
const PUNCTUATION_RE = String.raw`[^\p{L}\p{N}\s]+`
const WHITESPACE_RE = String.raw`\s+`
const TOKEN_RE = new RegExp(
  `(${WORD_RE}|${NUMBER_RE}|${PUNCTUATION_RE}|${WHITESPACE_RE})`,
  'gu', // find all matches
)
function tokenizeWords(text: string): string[] {
  return text.match(TOKEN_RE) ?? []
}

function buildWordDiffSegments(
  leftText: string,
  rightText: string,
): { leftSegments?: TextDiffSegment[]; rightSegments?: TextDiffSegment[] } {
  const leftTokens = tokenizeWords(leftText)
  const rightTokens = tokenizeWords(rightText)
  const patches: Patch[] = Array.from(diff(leftTokens, rightTokens))
  if (patches.length === 0) return {}

  const leftSegments: TextDiffSegment[] = []
  const rightSegments: TextDiffSegment[] = []
  let leftCursor = 0
  let rightCursor = 0

  for (const patch of patches) {
    const [leftStart, leftEnd, rightStart, rightEnd] = patch

    if (leftStart > leftCursor) {
      const equalText = leftTokens.slice(leftCursor, leftStart).join('')
      appendSegment(leftSegments, 'equal', equalText)
      appendSegment(rightSegments, 'equal', equalText)
    }

    if (leftEnd > leftStart) {
      const removedText = leftTokens.slice(leftStart, leftEnd).join('')
      appendSegment(leftSegments, 'removed', removedText)
    }

    if (rightEnd > rightStart) {
      const addedText = rightTokens.slice(rightStart, rightEnd).join('')
      appendSegment(rightSegments, 'added', addedText)
    }

    leftCursor = leftEnd
    rightCursor = rightEnd
  }

  // handle remaining tokens
  if (leftCursor < leftTokens.length || rightCursor < rightTokens.length) {
    const leftTailText = leftTokens.slice(leftCursor).join('')
    const rightTailText = rightTokens.slice(rightCursor).join('')

    if (leftTailText.length > 0) appendSegment(leftSegments, 'equal', leftTailText)
    if (rightTailText.length > 0) appendSegment(rightSegments, 'equal', rightTailText)
  }

  return {
    leftSegments: leftSegments.length > 0 ? leftSegments : undefined,
    rightSegments: rightSegments.length > 0 ? rightSegments : undefined,
  }
}

function appendSegment(segments: TextDiffSegment[], type: TextDiffSegmentType, text: string): void {
  if (text.length === 0) return
  const last = segments[segments.length - 1]
  if (last?.type === type) {
    last.text += text
    return
  }
  segments.push({ type, text })
}

function updateBothSideRowsIfIsModifiedBlock(
  patch: Patch,
  leftBlocks: ContractPlainTextBlock[],
  rightBlocks: ContractPlainTextBlock[],
  leftRows: ContractDiffRow[],
  rightRows: ContractDiffRow[],
  summary: ContractDiffSummary,
): boolean {
  const [leftStart, leftEnd, rightStart, rightEnd] = patch
  const leftCount = leftEnd - leftStart
  const rightCount = rightEnd - rightStart
  if (!isModifiedBlock(patch)) return false

  const pairCount = Math.min(leftCount, rightCount)

  // handle overlapping pairs
  for (let offset = 0; offset < pairCount; offset += 1) {
    const leftIndex = leftStart + offset
    const rightIndex = rightStart + offset
    const leftBlock = leftBlocks[leftIndex]
    const rightBlock = rightBlocks[rightIndex]
    if (!leftBlock || !rightBlock) continue

    const wordDiffSegments = buildWordDiffSegments(leftBlock.text, rightBlock.text)
    leftRows.push({
      id: `left-${leftIndex + 1}`,
      type: 'modified',
      lineNumber: leftIndex + 1,
      block: leftBlock,
      segments: wordDiffSegments.leftSegments,
    })
    rightRows.push({
      id: `right-${rightIndex + 1}`,
      type: 'modified',
      lineNumber: rightIndex + 1,
      block: rightBlock,
      segments: wordDiffSegments.rightSegments,
    })
    summary.modifiedCount += 1
  }

  // handle remaining left blocks
  for (let offset = pairCount; offset < leftCount; offset += 1) {
    const leftIndex = leftStart + offset
    const leftBlock = leftBlocks[leftIndex]
    if (!leftBlock) continue
    leftRows.push({
      id: `left-${leftIndex + 1}`,
      type: 'removed',
      lineNumber: leftIndex + 1,
      block: leftBlock,
    })
    summary.removedCount += 1
  }

  // handle remaining right blocks
  for (let offset = pairCount; offset < rightCount; offset += 1) {
    const rightIndex = rightStart + offset
    const rightBlock = rightBlocks[rightIndex]
    if (!rightBlock) continue
    rightRows.push({
      id: `right-${rightIndex + 1}`,
      type: 'added',
      lineNumber: rightIndex + 1,
      block: rightBlock,
    })
    summary.addedCount += 1
  }

  return true
}

function updateLeftRowIfIsDeletedBlock(
  patch: Patch,
  leftBlocks: ContractPlainTextBlock[],
  leftRows: ContractDiffRow[],
  summary: ContractDiffSummary,
): void {
  if (!isDeletedBlock(patch)) return

  const [leftStart, leftEnd, _rightStart, _rightEnd] = patch
  const leftCount = leftEnd - leftStart

  for (let offset = 0; offset < leftCount; offset += 1) {
    const leftIndex = leftStart + offset
    const leftBlock = leftBlocks[leftIndex]
    if (!leftBlock) continue
    leftRows.push({
      id: `left-${leftIndex + 1}`,
      type: 'removed',
      lineNumber: leftIndex + 1,
      block: leftBlock,
    })
    summary.removedCount += 1
  }
}

function updateRightRowIfIsAddedBlock(
  patch: Patch,
  rightBlocks: ContractPlainTextBlock[],
  rightRows: ContractDiffRow[],
  summary: ContractDiffSummary,
): void {
  if (!isAddedBlock(patch)) return

  const [_leftStart, _leftEnd, rightStart, rightEnd] = patch
  const rightCount = rightEnd - rightStart
  for (let offset = 0; offset < rightCount; offset += 1) {
    const rightIndex = rightStart + offset
    const rightBlock = rightBlocks[rightIndex]
    if (!rightBlock) continue
    rightRows.push({
      id: `right-${rightIndex + 1}`,
      type: 'added',
      lineNumber: rightIndex + 1,
      block: rightBlock,
    })
    summary.addedCount += 1
  }
}

function isModifiedBlock(patch: [number, number, number, number]): boolean {
  const [leftStart, leftEnd, rightStart, rightEnd] = patch
  const leftCount = leftEnd - leftStart
  const rightCount = rightEnd - rightStart
  // When leftCount and rightCount are greater than 0, it indicates a modified block.
  return leftCount > 0 && rightCount > 0
}

function isDeletedBlock(patch: [number, number, number, number]): boolean {
  const [_leftStart, _leftEnd, rightStart, rightEnd] = patch
  const rightCount = rightEnd - rightStart
  // When rightCount is 0, it indicates a deleted block.
  return rightCount === 0
}

function isAddedBlock(patch: [number, number, number, number]): boolean {
  const [leftStart, leftEnd, _rightStart, _rightEnd] = patch
  const leftCount = leftEnd - leftStart
  // When leftCount is 0, it indicates a added block.
  return leftCount === 0
}
