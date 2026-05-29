import { ref } from 'vue'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import type { TemplateCatalogueSearchRequest } from '@/models/requests/template-catalogue-integration-request'

type TemplateCatalogueListQuery = Partial<
  Pick<TemplateCatalogueSearchRequest, 'did' | 'document_number' | 'version' | 'name' | 'description'>
>

export function useTemplateCatalogueList(defaultQuery: TemplateCatalogueListQuery = {}) {
  const templates = ref<TemplateResourcesItem[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh(query: TemplateCatalogueListQuery = {}) {
    loading.value = true
    error.value = null
    try {
      const response = await templateCatalogueIntegrationService.search_template({
        offset: 0,
        limit: 0,
        did: query.did ?? defaultQuery.did,
        document_number: query.document_number ?? defaultQuery.document_number,
        version: query.version ?? defaultQuery.version,
        name: query.name ?? defaultQuery.name,
        description: query.description ?? defaultQuery.description,
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
