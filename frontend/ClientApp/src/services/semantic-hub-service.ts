import http from '@/api/http'

/**
 * Semantic Hub clause catalog: the palette listing of typed clause
 * NodeShapes plus the raw SHACL Turtle they live in — forms are generated
 * client-side from the Turtle (shacl-form). Public endpoint (no auth),
 * like resolve_context — see backend/design/semantic_hub.go "clauses".
 */
export interface ClauseCatalogType {
  type: string
  label: string
  /** The NodeShape's IRI within the shapes graph. */
  shape: string
}

export interface ClauseCatalogResponse {
  version: number
  clauses: ClauseCatalogType[]
  shapes: string
}

export async function getClauseCatalog(): Promise<ClauseCatalogResponse> {
  return http.get('/semantic/clauses').then((res) => res.data)
}

/** One (name, kind) hub entry summary (GET /semantic/schema/list). */
export interface SemanticSchemaListEntry {
  name: string
  kind: string
  media_type: string
  /** 0 when no version is active (registered without activation). */
  active_version: number
  latest_version: number
  updated_at: string
}

/** One stored schema version (GET /semantic/schema/versions | /retrieve). */
export interface SemanticSchemaItem {
  name: string
  version: number
  kind: string
  media_type: string
  content: string
  active: boolean
  created_by: string
  created_at: string
}

export interface RegisterSchemaPayload {
  name: string
  kind: string
  media_type: string
  /** Inline content; omit when source_url is given. */
  content?: string
  /**
   * Fetch the schema from this URL (http/https, follows redirects) instead of
   * inline content. A DCS hub anchor (/semantic/shapes/{name}?version=N) is
   * unwrapped to the document it serves.
   */
  source_url?: string
  /**
   * Register at exactly this version instead of the next one — how a shape
   * library another instance published is installed here under the number
   * that instance assigned it, so a template pinning ?version=N resolves the
   * same graph on both hubs (ADR-8). Rejected when the version already exists.
   */
  version?: number
  activate: boolean
}

export async function listSchemas(): Promise<SemanticSchemaListEntry[]> {
  return http.get('/semantic/schema/list').then((res) => res.data)
}

export async function getSchemaVersions(name: string, kind: string): Promise<SemanticSchemaItem[]> {
  return http.get('/semantic/schema/versions', { params: { name, kind } }).then((res) => res.data)
}

export async function registerSchema(
  payload: RegisterSchemaPayload,
): Promise<{ name: string; version: number; kind: string; active: boolean }> {
  return http.post('/semantic/schema/register', payload).then((res) => res.data)
}

export async function rollbackSchema(
  name: string,
  kind: string,
  version: number,
): Promise<{ name: string; version: number; kind: string; active: boolean }> {
  return http.post('/semantic/schema/rollback', { name, kind, version }).then((res) => res.data)
}
