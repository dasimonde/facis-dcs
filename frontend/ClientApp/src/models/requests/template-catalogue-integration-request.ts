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
}

// ---- Participant management ----

export interface TemplateCatalogueGetCurrentParticipantRequest { }
export interface TemplateCatalogueGetCurrentParticipantSummaryRequest { }
export interface TemplateCatalogueGetOtherParticipantsRequest { }
export interface TemplateCatalogueCreateParticipantRequest {
  legal_name: string
  registration_number: string
  lei_code: string
  ethereum_address: string
  headquarter_address: ParticipantHeadquarterAddress
  legal_address: ParticipantLegalAddress
  terms_and_conditions: string
}

export interface TemplateCatalogueUpdateParticipantRequest extends TemplateCatalogueCreateParticipantRequest { }

export interface TemplateCatalogueDeleteParticipantRequest { }

// ---- Service offering management ----

export interface TemplateCatalogueGetCurrentServiceOfferingRequest { }
export interface TemplateCatalogueCreateServiceOfferingRequest {
  keywords: string[]
  description: string
  end_point_url: string
  terms_and_conditions: string
}

export interface TemplateCatalogueUpdateServiceOfferingRequest extends TemplateCatalogueCreateServiceOfferingRequest { }

export interface TemplateCatalogueDeleteServiceOfferingRequest { }

