import type {
  TemplateResource,
  TemplateResourcesItem,
} from '@/modules/template-catalogue/models/template-resource'
import type {
  ParticipantHeadquarterAddress,
  ParticipantLegalAddress,
  Participant,
} from '@/modules/template-catalogue/models/participant'

// ---- Template retrieval ----

export interface TemplateCatalogueRetrieveResponse {
  totalCount: number
  items: TemplateResourcesItem[]
}

export interface TemplateCatalogueRetrieveByIdResponse extends TemplateResource { }

// ---- Participant management ----

export interface TemplateCatalogueCreateParticipantResponse {
  id: string
}

export interface TemplateCatalogueGetCurrentParticipantResponse {
  legal_name: string
  registration_number: string
  lei_code: string
  ethereum_address: string
  headquarter_address: ParticipantHeadquarterAddress
  legal_address: ParticipantLegalAddress
  terms_and_conditions: string
}

export type TemplateCatalogueGetCurrentParticipantSummaryResponse = Participant
export type TemplateCatalogueGetOtherParticipantsResponse = Participant[]

export interface TemplateCatalogueUpdateParticipantResponse {
  id: string
}

export interface TemplateCatalogueDeleteParticipantResponse {
  id: string
}

// ---- Service offering management ----

export interface TemplateCatalogueCreateServiceOfferingResponse {
  id: string
}

export interface TemplateCatalogueGetCurrentServiceOfferingResponse {
  keywords: string[]
  description: string
  end_point_url: string
  terms_and_conditions: string
}

export interface TemplateCatalogueUpdateServiceOfferingResponse {
  id: string
}

export interface TemplateCatalogueDeleteServiceOfferingResponse {
  id: string
}

