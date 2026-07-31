import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { type ClauseCatalogType, getClauseCatalog } from '@/services/semantic-hub-service'

/**
 * Process-wide cache of the Semantic Hub's active clause catalog
 * (GET /semantic/clauses, ADR-10) shared by the template builder's palette,
 * the add-block modal, and the editor canvas. refresh() refetches — the
 * add-block modal and the clauses tab call it on open, so a clause-catalog
 * version registered+activated in the hub is usable in the builder
 * immediately, without a reload or redeploy.
 */
export const useClauseCatalogStore = defineStore('clause-catalog', () => {
  const clauses = ref<ClauseCatalogType[]>([])
  const shapes = ref('')
  const version = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  let inflight: Promise<void> | null = null

  async function refresh(): Promise<void> {
    if (inflight) return inflight
    loading.value = true
    error.value = null
    inflight = getClauseCatalog()
      .then((catalog) => {
        clauses.value = catalog.clauses
        shapes.value = catalog.shapes
        version.value = catalog.version
      })
      .catch((err: unknown) => {
        error.value = err instanceof Error ? err.message : 'Failed to load the clause catalog'
      })
      .finally(() => {
        loading.value = false
        inflight = null
      })
    return inflight
  }

  /** Loads once; later callers get the cache. Use refresh() to force a refetch. */
  async function ensureLoaded(): Promise<void> {
    if (clauses.value.length || loading.value) return
    return refresh()
  }

  const byType = computed(() => {
    const map = new Map<string, ClauseCatalogType>()
    for (const clause of clauses.value) map.set(clause.type, clause)
    return map
  })

  function labelFor(clauseType: string): string {
    return byType.value.get(clauseType)?.label ?? clauseType.replace(/^dcs:/, '')
  }

  return { clauses, shapes, version, loading, error, refresh, ensureLoaded, byType, labelFor }
})
