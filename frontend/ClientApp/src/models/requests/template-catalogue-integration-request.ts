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
  version?: number
  name?: string
  description?: string
  offset: number
  limit: number
}
