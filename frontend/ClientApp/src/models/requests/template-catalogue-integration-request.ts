import type {
  ParticipantHeadquarterAddress,
  ParticipantLegalAddress,
} from '@/modules/template-catalogue/models/participant'

// ---- Template retrieval ----

export interface TemplateCatalogueRetrieveRequest {
  offset: number
  limit: number
}

export interface TemplateCatalogueRetrieveByIdRequest {
  did: string
  version: number
}

export interface TemplateCatalogueSearchRequest {
  did?: string
  document_number?: string
  version?: number
  name?: string
  description?: string
  offset: number
  limit: number
}

// ---- Participant management ----

export type TemplateCatalogueGetCurrentParticipantRequest = Record<string, unknown>
export type TemplateCatalogueGetCurrentParticipantSummaryRequest = Record<string, unknown>
export type TemplateCatalogueGetOtherParticipantsRequest = Record<string, unknown>
export interface TemplateCatalogueCreateParticipantRequest {
  legal_name: string
  registration_number: string
  lei_code: string
  ethereum_address: string
  headquarter_address: ParticipantHeadquarterAddress
  legal_address: ParticipantLegalAddress
  terms_and_conditions: string
}

export type TemplateCatalogueUpdateParticipantRequest = TemplateCatalogueCreateParticipantRequest

export type TemplateCatalogueDeleteParticipantRequest = Record<string, unknown>

// ---- Service offering management ----

export type TemplateCatalogueGetCurrentServiceOfferingRequest = Record<string, unknown>
export interface TemplateCatalogueCreateServiceOfferingRequest {
  keywords: string[]
  description: string
  end_point_url: string
  terms_and_conditions: string
}

export type TemplateCatalogueUpdateServiceOfferingRequest = TemplateCatalogueCreateServiceOfferingRequest

export type TemplateCatalogueDeleteServiceOfferingRequest = Record<string, unknown>
