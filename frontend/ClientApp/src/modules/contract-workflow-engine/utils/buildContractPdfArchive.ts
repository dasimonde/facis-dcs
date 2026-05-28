import type { Contract } from '@/models/contract/contract'
import type {
  ContractPdfArchive,
  ContractPdfArchiveManifest,
  ContractPdfEmbeddedContract,
} from '@/types/contract-pdf-archive'
import {
  ContractPdfArchiveFileNames,
  ContractPdfArchiveFileTypes,
  ContractPdfArchiveManifestVersions,
  ContractPdfRepresentations,
  PdfArchiveMediaTypes,
} from '@/types/contract-pdf-archive'

/**
 * Builds a PDF archive for a contract
 * @param contract - The contract to build the archive for
 * @returns The PDF archive
 */
export async function buildContractPdfArchive(contract: Contract): Promise<ContractPdfArchive> {
  const contractPayload: ContractPdfEmbeddedContract = {
    did: contract.did,
    name: contract.name,
    contract_version: contract.contract_version,
    contract_data: contract.contract_data,
  }

  const contractJson = JSON.stringify(contractPayload)
  const contractSha256 = await sha256Hex(contractJson)

  const manifest: ContractPdfArchiveManifest = {
    manifestVersion: ContractPdfArchiveManifestVersions.v1,
    contractRepresentation: ContractPdfRepresentations.json,
    exportedAt: new Date().toISOString(),
    contract: {
      did: contract.did,
      contractVersion: contract.contract_version,
      name: contract.name,
    },
    files: [
      {
        path: ContractPdfArchiveFileNames.contractJson,
        type: ContractPdfArchiveFileTypes.contract,
        mediaType: PdfArchiveMediaTypes.applicationJson,
        sha256: contractSha256,
      },
    ],
  }

  return {
    contractBytes: encodeUtf8(contractJson),
    manifestBytes: encodeUtf8(JSON.stringify(manifest)),
  }
}

async function sha256Hex(text: string): Promise<string> {
  const bytes = encodeUtf8(text)
  // crypto.subtle only works in secure contexts (https or localhost)
  const digest = await crypto.subtle.digest('SHA-256', toArrayBuffer(bytes))
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('')
}

function encodeUtf8(text: string): Uint8Array {
  return new TextEncoder().encode(text)
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
}
