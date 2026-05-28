import type { ContractPlainTextBlock } from '../composables/useContractPlainTextConverter'
import { isSectionPlainTextBlock, isTextPlainTextBlock } from '../composables/useContractPlainTextConverter'
import type { ContentText, Style, StyleDictionary, StyleReference, TDocumentDefinitions } from 'pdfmake/interfaces'

type PdfStyles = StyleDictionary
type PdfContent = ContentText[]

export interface PdfDataResult {
  content: PdfContent
  styles: PdfStyles
  version: TDocumentDefinitions['version']
  subset: TDocumentDefinitions['subset']
  tagged: boolean
  // Controls whether the document title should be displayed in the window title of the PDF viewer
  displayTitle: boolean
}

/** Do not strip leading, trailing, or repeated spaces */
const preserveWhitespace: Style = {
  preserveLeadingSpaces: true,
  preserveTrailingSpaces: true,
} as const

const pdfStyles: PdfStyles = {
  section1: {
    fontSize: 16,
    bold: true,
    color: '#1f2937',
    margin: [0, 0, 0, 6],
    ...preserveWhitespace,
  },
  section2: {
    fontSize: 14,
    bold: true,
    color: '#1f2937',
    margin: [0, 0, 0, 6],
    ...preserveWhitespace,
  },
  section3: {
    fontSize: 12,
    bold: true,
    color: '#1f2937',
    margin: [0, 0, 0, 6],
    ...preserveWhitespace,
  },
  text: {
    fontSize: 12,
    color: '#374151',
    margin: [0, 0, 0, 6],
    ...preserveWhitespace,
  },
}

export function toPdfData(blocks: ContractPlainTextBlock[]): PdfDataResult {
  const content: PdfContent = []

  for (const block of blocks) {
    if (isSectionPlainTextBlock(block)) {
      content.push({
        text: block.text ?? '',
        style: getSectionStyle(block.level),
      })
      continue
    }

    if (isTextPlainTextBlock(block)) {
      content.push({
        text: block.text ?? '',
        style: 'text',
      })
    }
  }

  return {
    content,
    styles: pdfStyles,
    version: '1.7ext3',
    subset: 'PDF/A-3a',
    tagged: true,
    displayTitle: true,
  }
}

function getSectionStyle(level: number): StyleReference {
  if (level <= 1) return 'section1'
  if (level === 2) return 'section2'
  return 'section3'
}
