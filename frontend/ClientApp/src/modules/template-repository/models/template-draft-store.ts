import type { ContractTemplateResponsible } from '@/models/contract-template/contract-template-responsible'
import type { DcsBlock, DcsContractDataObject, DcsContractField, DcsLayoutNode, OdrlRule } from '@/models/dcs-jsonld'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { MetaData, TemplateTypeValue } from '@template-repository/models/contract-template'

export const TEMPLATE_DATA_VERSIONS = [1] as const
export type TemplateDataVersion = (typeof TEMPLATE_DATA_VERSIONS)[number]

interface TemplateDraftState {
  did: string | null
  /** The document's @id — its dereferenceable resource IRI; authored fragments anchor to it. */
  documentIri: string | null
  name: string
  description: string
  templateDataVersion: TemplateDataVersion
  /** JSON-LD blocks (self-contained sections/clauses/text). */
  blocks: DcsBlock[]
  /** JSON-LD layout tree. */
  layout: DcsLayoutNode[]
  /** JSON-LD field declarations referenced by clauses and domain data. */
  contractFields: DcsContractField[]
  /** Typed business objects that bind properties to the declared fields. */
  contractData: DcsContractDataObject[]
  /** JSON-LD ODRL policies (operator constraints). */
  policies: OdrlRule[]
  customMetaData: MetaData[]
  templateType: TemplateTypeValue
  state: ContractTemplateState | undefined
  version: number | null
  updated_at: string | null
  created_by: string
  responsible: ContractTemplateResponsible | null
  workflow: 'contract' | 'template'
}

/** Block types in JSON-LD @type notation. */
export type NewBlockType = 'dcs:Section' | 'dcs:TextBlock' | 'dcs:Clause'

/** Payload for adding a new block. */
export interface AddBlockPayload {
  blockType: NewBlockType
  text?: string
  title?: string
  // #### For Clause ####
  clauseBlockId?: string
  content?: import('@/models/dcs-jsonld').DcsContentSegment[]
}

export interface AddBlockOptions {
  addToOutline?: boolean
}

export type { TemplateDraftState }
