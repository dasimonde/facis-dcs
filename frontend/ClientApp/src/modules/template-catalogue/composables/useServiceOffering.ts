import { ref } from 'vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateCatalogueCreateServiceOfferingRequest, TemplateCatalogueUpdateServiceOfferingRequest } from '@/models/requests/template-catalogue-integration-request'
import type { TemplateCatalogueGetCurrentServiceOfferingResponse } from '@/models/responses/template-catalogue-integration-response'

export function useServiceOffering() {
  const currentServiceOffering = ref<TemplateCatalogueGetCurrentServiceOfferingResponse | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadCurrent() {
    loading.value = true
    error.value = null
    try {
      currentServiceOffering.value = await templateCatalogueIntegrationService.get_current_service_offering()
    } catch (e: any) {
      error.value = e?.message || 'Error loading service offering'
      currentServiceOffering.value = null
    } finally {
      loading.value = false
    }
  }

  async function createServiceOffering(request: TemplateCatalogueCreateServiceOfferingRequest) {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.create_service_offering(request)
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error creating service offering'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function updateServiceOffering(request: TemplateCatalogueUpdateServiceOfferingRequest) {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.update_service_offering(request)
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error updating service offering'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function deleteServiceOffering() {
    loading.value = true
    error.value = null
    try {
      await templateCatalogueIntegrationService.delete_service_offering()
      await loadCurrent()
    } catch (e: any) {
      error.value = e?.message || 'Error deleting service offering'
      throw e
    } finally {
      loading.value = false
    }
  }

  return { currentServiceOffering, loading, error, loadCurrent, createServiceOffering, updateServiceOffering, deleteServiceOffering }
}

