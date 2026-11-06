import type { PartyType } from '@/types/party-type'
export interface TemplateCatalogueRetrieveRequest {
  offset: number
  limit: number
}

export interface TemplateCatalogueRetrieveByIdRequest {
  did: string
  version: number
}

export interface TemplateCatalogueSearchRequest {
  party_type?: PartyType
  did?: string
  version?: number
  name?: string
  description?: string
  offset: number
  limit: number
}
