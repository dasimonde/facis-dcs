import http from '@/api/http'
import type {
  TemplateCatalogueRetrieveByIdRequest,
  TemplateCatalogueRetrieveRequest,
  TemplateCatalogueSearchRequest,
} from '@/models/requests/template-catalogue-integration-request'
import type {
  TemplateCatalogueRetrieveByIdResponse,
  TemplateCatalogueRetrieveResponse,
} from '@/models/responses/template-catalogue-integration-response'

// Template Catalogue Integration Service (TR <-> XFSC Catalogue)
export const templateCatalogueIntegrationService = {
  async retrieve_template(request: TemplateCatalogueRetrieveRequest): Promise<TemplateCatalogueRetrieveResponse> {
    return http
      .get<TemplateCatalogueRetrieveResponse>('/catalogue/template/retrieve', { params: request })
      .then((res) => res.data)
  },

  async retrieve_template_by_id(
    request: TemplateCatalogueRetrieveByIdRequest,
  ): Promise<TemplateCatalogueRetrieveByIdResponse | null> {
    return http
      .get<TemplateCatalogueRetrieveByIdResponse | null>(`/catalogue/template/retrieve/${request.did}`, {
        params: {
          version: request.version,
        },
      })
      .then((res) => res.data ?? null)
  },

  async search_template(request: TemplateCatalogueSearchRequest): Promise<TemplateCatalogueRetrieveResponse> {
    return http
      .get<TemplateCatalogueRetrieveResponse>('/catalogue/template/search', { params: request })
      .then((res) => res.data)
  },
}
