import type { Contract } from '@/models/contract/contract'

/** Manifest schema version values */
export const ContractPdfArchiveManifestVersions = {
  v1: 1,
} as const

/** Machine-readable encoding */
export const ContractPdfRepresentations = {
  json: 'json',
  jsonLd: 'json-ld',
} as const

/** Embedded file names inside the PDF */
export const ContractPdfArchiveFileNames = {
  contractJson: 'contract.json',
  manifestJson: 'manifest.json',
} as const

/**
 * Identifies each embedded attachment listed in manifest.json.
 * Currently only support `contract`.
 * Future extensions may include attachments such as `parties` metadata JSON.
 */
export const ContractPdfArchiveFileTypes = {
  contract: 'contract',
} as const

export type ContractPdfArchiveManifestVersion =
  (typeof ContractPdfArchiveManifestVersions)[keyof typeof ContractPdfArchiveManifestVersions]
export type ContractPdfRepresentation = (typeof ContractPdfRepresentations)[keyof typeof ContractPdfRepresentations]
export type ContractPdfArchiveFileType = (typeof ContractPdfArchiveFileTypes)[keyof typeof ContractPdfArchiveFileTypes]

/** Body of embedded contract.json */
export type ContractPdfEmbeddedContract = Pick<Contract, 'did' | 'name' | 'contract_version' | 'contract_data'>

export const PdfArchiveMediaTypes = {
  applicationJson: 'application/json',
  applicationLdJson: 'application/ld+json',
} as const

export type PdfArchiveMediaType = (typeof PdfArchiveMediaTypes)[keyof typeof PdfArchiveMediaTypes]

export interface ContractPdfArchiveManifestFile {
  path: string
  type: ContractPdfArchiveFileType
  mediaType: 'application/json' | 'application/ld+json'
  sha256: string
}

export interface ContractPdfArchiveManifestContractRef {
  did: string
  contractVersion: number
  name?: string
}

/** Embedded manifest.json */
export interface ContractPdfArchiveManifest {
  manifestVersion: ContractPdfArchiveManifestVersion
  contractRepresentation: ContractPdfRepresentation
  exportedAt: string
  contract: ContractPdfArchiveManifestContractRef
  files: ContractPdfArchiveManifestFile[]
}

/** UTF-8 payloads ready for PDF embedding */
export interface ContractPdfArchive {
  /** The bytes of the ContractPdfEmbeddedContract JSON payload */
  contractBytes: Uint8Array
  /** The bytes of the ContractPdfArchiveManifest JSON payload */
  manifestBytes: Uint8Array
}
