import pdfMake from 'pdfmake/build/pdfmake'
import pdfFonts from 'pdfmake/build/vfs_fonts'
import type { Attachment, ContentText, TDocumentDefinitions } from 'pdfmake/interfaces'
import {
  ContractPdfArchiveFileNames,
  PdfArchiveMediaTypes,
  type ContractPdfArchive,
  type PdfArchiveMediaType,
} from '@/types/contract-pdf-archive'
import type { PdfDataResult } from './contractPdfConverter'

type PdfMakeWithVfs = typeof pdfMake & {
  /** virtual file system */
  vfs?: Record<string, string>
}

interface ExportOptions {
  displayTitleInContent?: boolean
  /** Embedded JSON attachments */
  archive?: ContractPdfArchive
}

let isPdfMakeConfigured = false

function configurePdfMake(): void {
  if (isPdfMakeConfigured) return

  const pdfMakeInstance = pdfMake as PdfMakeWithVfs
  const fontFileSystem = pdfFonts as unknown as { vfs?: Record<string, string> }
  if (fontFileSystem.vfs) {
    // Register the bundled virtual font filesystem (Roboto, etc.) with pdfMake
    // so createPdf() can resolve fonts in the browser build.
    pdfMakeInstance.vfs = fontFileSystem.vfs
  }

  isPdfMakeConfigured = true
}

function toPdfDocumentDefinition(
  pdfData: PdfDataResult,
  title = 'Contract Document',
  options?: ExportOptions,
): TDocumentDefinitions {
  const displayTitleInContent = options?.displayTitleInContent ?? false
  const content = displayTitleInContent ? [getTitleNode(title), ...pdfData.content] : pdfData.content
  return {
    content: content,
    styles: pdfData.styles,
    info: {
      title,
    },
    version: pdfData.version,
    subset: pdfData.subset,
    tagged: pdfData.tagged,
    displayTitle: pdfData.displayTitle,
    files: options?.archive ? getAttachments(options.archive) : undefined,
  }
}

export function downloadContractPdf(
  pdfData: PdfDataResult,
  filename = 'contract.pdf',
  title?: string,
  options?: ExportOptions,
): void {
  configurePdfMake()

  const documentDefinition = toPdfDocumentDefinition(pdfData, title, options)
  void pdfMake.createPdf(documentDefinition).download(filename)
}

function getTitleNode(title: string): ContentText {
  return {
    text: title,
    style: 'docTitle',
    alignment: 'center',
    margin: [0, 0, 0, 12],
  }
}
/**
 *  Maps archive bytes to embedded files
 */
function getAttachments(archive: ContractPdfArchive): Record<string, Attachment> {
  return {
    [ContractPdfArchiveFileNames.contractJson]: {
      src: bytesToDataUri(archive.contractBytes, PdfArchiveMediaTypes.applicationJson),
      name: ContractPdfArchiveFileNames.contractJson,
      description: 'Machine-readable contract',
    },
    [ContractPdfArchiveFileNames.manifestJson]: {
      src: bytesToDataUri(archive.manifestBytes, PdfArchiveMediaTypes.applicationJson),
      name: ContractPdfArchiveFileNames.manifestJson,
      description: 'PDF archive manifest',
    },
  }
}

function bytesToDataUri(bytes: Uint8Array, mimeType: PdfArchiveMediaType): string {
  return `data:${mimeType};base64,${uint8ArrayToBase64(bytes)}`
}

function uint8ArrayToBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  // Convert binary string to base64
  return btoa(binary)
}
