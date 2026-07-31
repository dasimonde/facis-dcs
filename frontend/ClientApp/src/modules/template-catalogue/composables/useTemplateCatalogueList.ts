import { ref } from 'vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateResourcesItem } from '@template-catalogue/models/template-resource'

interface TemplateCatalogueListQuery {
  offset?: number
  limit?: number
}

export function useTemplateCatalogueList(defaultQuery: TemplateCatalogueListQuery = {}) {
  const templates = ref<TemplateResourcesItem[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh(query: TemplateCatalogueListQuery = {}) {
    loading.value = true
    error.value = null
    try {
      const response = await templateCatalogueIntegrationService.retrieve_template({
        offset: query.offset ?? defaultQuery.offset ?? 0,
        limit: query.limit ?? defaultQuery.limit ?? 0,
      })
      templates.value = response?.items ?? []
    } catch (e: unknown) {
      error.value = e instanceof Error && e.message ? e.message : 'Error loading catalogue templates'
      templates.value = []
    } finally {
      loading.value = false
    }
  }

  return { templates, loading, error, refresh }
}
