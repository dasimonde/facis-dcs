export interface Participant {
  legal_name?: string
  registration_number?: string
  lei_code?: string
  ethereum_address?: string
  headquarter_address?: ParticipantHeadquarterAddress
  legal_address?: ParticipantLegalAddress
  terms_and_conditions?: string
}

export interface ParticipantHeadquarterAddress {
  country?: string
  street_address?: string
  postal_code?: string
  locality?: string
}

export interface ParticipantLegalAddress {
  country?: string
  street_address?: string
  postal_code?: string
  locality?: string
}
