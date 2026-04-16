import http from '@/api/http'
import type {
  TemplateCatalogueCreateParticipantRequest,
  TemplateCatalogueDeleteParticipantRequest,
  TemplateCatalogueGetCurrentParticipantRequest,
  TemplateCatalogueGetCurrentParticipantSummaryRequest,
  TemplateCatalogueGetOtherParticipantsRequest,
  TemplateCatalogueRetrieveByIdRequest,
  TemplateCatalogueRetrieveRequest,
  TemplateCatalogueUpdateParticipantRequest,
  TemplateCatalogueCreateServiceOfferingRequest,
  TemplateCatalogueDeleteServiceOfferingRequest,
  TemplateCatalogueGetCurrentServiceOfferingRequest,
  TemplateCatalogueUpdateServiceOfferingRequest,
} from '@/models/requests/template-catalogue-integration-request'
import type {
  TemplateCatalogueCreateParticipantResponse,
  TemplateCatalogueDeleteParticipantResponse,
  TemplateCatalogueGetCurrentParticipantResponse,
  TemplateCatalogueGetCurrentParticipantSummaryResponse,
  TemplateCatalogueGetOtherParticipantsResponse,
  TemplateCatalogueRetrieveByIdResponse,
  TemplateCatalogueRetrieveResponse,
  TemplateCatalogueUpdateParticipantResponse,
  TemplateCatalogueCreateServiceOfferingResponse,
  TemplateCatalogueDeleteServiceOfferingResponse,
  TemplateCatalogueGetCurrentServiceOfferingResponse,
  TemplateCatalogueUpdateServiceOfferingResponse,
} from '@/models/responses/template-catalogue-integration-response'

// Template Catalogue Integration Service (TR <-> XFSC Catalogue)
export const templateCatalogueIntegrationService = {
  // ---- Participant ----
  async create_participant(
    request: TemplateCatalogueCreateParticipantRequest,
  ): Promise<TemplateCatalogueCreateParticipantResponse> {
    return http.post<TemplateCatalogueCreateParticipantResponse>('/catalogue/participant/create', request).then((res) => res.data)
  },

  async get_current_participant(
    _request: TemplateCatalogueGetCurrentParticipantRequest = {},
  ): Promise<TemplateCatalogueGetCurrentParticipantResponse | null> {
    return http
      .get<TemplateCatalogueGetCurrentParticipantResponse>('/catalogue/participant/current')
      .then((res) => res.data)
      .catch((err: any) => {
        if (err?.response?.status === 404) {
          return null
        }
        throw err
      })
  },

  async get_current_participant_summary(
    _request: TemplateCatalogueGetCurrentParticipantSummaryRequest = {},
  ): Promise<TemplateCatalogueGetCurrentParticipantSummaryResponse | null> {
    return http
      .get<TemplateCatalogueGetCurrentParticipantSummaryResponse>('/catalogue/participant/current/summary')
      .then((res) => res.data)
      .catch((err: any) => {
        if (err?.response?.status === 404) {
          return null
        }
        throw err
      })
  },

  async get_other_participants(
    _request: TemplateCatalogueGetOtherParticipantsRequest = {},
  ): Promise<TemplateCatalogueGetOtherParticipantsResponse> {
    return http
      .get<TemplateCatalogueGetOtherParticipantsResponse>('/catalogue/participant/others')
      .then((res) => res.data)
  },

  async update_participant(
    request: TemplateCatalogueUpdateParticipantRequest,
  ): Promise<TemplateCatalogueUpdateParticipantResponse> {
    return http.put<TemplateCatalogueUpdateParticipantResponse>('/catalogue/participant/update', request).then((res) => res.data)
  },

  async delete_participant(
    _request: TemplateCatalogueDeleteParticipantRequest = {},
  ): Promise<TemplateCatalogueDeleteParticipantResponse> {
    return http.delete<TemplateCatalogueDeleteParticipantResponse>('/catalogue/participant/delete').then((res) => res.data)
  },

  // ---- Service offering ----
  async create_service_offering(
    request: TemplateCatalogueCreateServiceOfferingRequest,
  ): Promise<TemplateCatalogueCreateServiceOfferingResponse> {
    return http
      .post<TemplateCatalogueCreateServiceOfferingResponse>('/catalogue/service-offering/create', request)
      .then((res) => res.data)
  },

  async get_current_service_offering(
    _request: TemplateCatalogueGetCurrentServiceOfferingRequest = {},
  ): Promise<TemplateCatalogueGetCurrentServiceOfferingResponse | null> {
    return http
      .get<TemplateCatalogueGetCurrentServiceOfferingResponse>('/catalogue/service-offering/current')
      .then((res) => res.data)
      .catch((err: any) => {
        if (err?.response?.status === 404) {
          return null
        }
        throw err
      })
  },

  async update_service_offering(
    request: TemplateCatalogueUpdateServiceOfferingRequest,
  ): Promise<TemplateCatalogueUpdateServiceOfferingResponse> {
    return http
      .put<TemplateCatalogueUpdateServiceOfferingResponse>('/catalogue/service-offering/update', request)
      .then((res) => res.data)
  },

  async delete_service_offering(
    _request: TemplateCatalogueDeleteServiceOfferingRequest = {},
  ): Promise<TemplateCatalogueDeleteServiceOfferingResponse> {
    return http
      .delete<TemplateCatalogueDeleteServiceOfferingResponse>('/catalogue/service-offering/delete')
      .then((res) => res.data)
  },

  // ---- Template ----
  async retrieve_template(
    request: TemplateCatalogueRetrieveRequest,
  ): Promise<TemplateCatalogueRetrieveResponse> {
    return http
      .get<TemplateCatalogueRetrieveResponse>('/catalogue/template/retrieve', { params: request })
      .then((res) => res.data)
      .catch(() => ({ totalCount: 0, items: [] } as TemplateCatalogueRetrieveResponse))
  },

  async retrieve_template_by_id(
    request: TemplateCatalogueRetrieveByIdRequest,
  ): Promise<TemplateCatalogueRetrieveByIdResponse | null> {
    return http
      .get<TemplateCatalogueRetrieveByIdResponse>(`/catalogue/template/retrieve/${encodeURIComponent(request.did)}`)
      .then((res) => res.data)
      .catch(() => null)
  },
}

