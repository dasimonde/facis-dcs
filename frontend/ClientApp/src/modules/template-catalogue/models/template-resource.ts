import type { DcsTemplateData } from '@/models/dcs-jsonld'

export interface TemplateResource {
  did: string
  version: number
  name?: string
  description?: string
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
  template_type?: string
  participant_id?: string
  created_at?: string
  updated_at?: string
}
