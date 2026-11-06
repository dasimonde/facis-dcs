import type { DcsTemplateData } from '@/models/dcs-jsonld'
import type { PartyType } from '@/types/party-type'

export interface TemplateResource {
  did: string
  version: number
  name?: string
  description?: string
  party_type?: PartyType
  template_type?: string
  participant_id?: string
  created_at?: string
  updated_at?: string
  template_data?: DcsTemplateData
}

export interface TemplateResourcesItem {
  did: string
  version: number
  name?: string
  description?: string
  party_type?: PartyType
  template_type?: string
  participant_id?: string
  created_at?: string
  updated_at?: string
}
