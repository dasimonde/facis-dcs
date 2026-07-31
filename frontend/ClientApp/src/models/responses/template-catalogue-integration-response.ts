import type { TemplateResource, TemplateResourcesItem } from '@template-catalogue/models/template-resource'

export interface TemplateCatalogueRetrieveResponse {
  totalCount: number
  items: TemplateResourcesItem[]
}

export type TemplateCatalogueRetrieveByIdResponse = TemplateResource
