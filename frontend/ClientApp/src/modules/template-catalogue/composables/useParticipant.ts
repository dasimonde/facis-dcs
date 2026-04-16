import { ref } from 'vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateCatalogueCreateParticipantRequest, TemplateCatalogueUpdateParticipantRequest } from '@/models/requests/template-catalogue-integration-request'
import type { TemplateCatalogueGetCurrentParticipantResponse } from '@/models/responses/template-catalogue-integration-response'

export function useParticipant() {
  const currentParticipant = ref<TemplateCatalogueGetCurrentParticipantResponse | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadCurrent() {
    loading.value = true
    error.value = null
    try {
      currentParticipant.value = await templateCatalogueIntegrationService.get_current_participant()
    } catch (e: any) {
      error.value = e?.message || 'Error loading participant'
      currentParticipant.value = null
    } finally {
      loading.value = false
    }
  }

  async function createParticipant(request: TemplateCatalogueCreateParticipantRequest) {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.create_participant(request)
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error creating participant'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function updateParticipant(request: TemplateCatalogueUpdateParticipantRequest) {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.update_participant(request)
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error updating participant'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function deleteParticipant() {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.delete_participant()
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error deleting participant'
      throw e
    } finally {
      loading.value = false
    }
  }

  return { currentParticipant, loading, error, loadCurrent, createParticipant, updateParticipant, deleteParticipant }
}

