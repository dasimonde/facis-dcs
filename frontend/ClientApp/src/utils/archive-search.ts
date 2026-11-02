import type { ArchivedContract, ArchiveSearchQuery } from '@/services/archive-service'

/** A named archive search the user kept (DCS-FR-CSA-22). */
export interface SavedArchiveQuery {
  name: string
  query: ArchiveSearchQuery
}

const STORAGE_KEY = 'dcs.archive.saved-queries'

/** The filters that are set; empty ones are left out of the request. */
export function activeFilters(query: ArchiveSearchQuery): ArchiveSearchQuery {
  return Object.fromEntries(
    Object.entries(query).filter(([, value]) =>
      typeof value === 'string' ? value.trim() !== '' : value !== undefined && value !== null,
    ),
  )
}

/**
 * Saved queries live in this browser only. Storage that is unavailable
 * (private window, blocked site data) gives an empty list.
 */
export function loadSavedQueries(storage: Storage = localStorage): SavedArchiveQuery[] {
  try {
    const parsed: unknown = JSON.parse(storage.getItem(STORAGE_KEY) ?? '[]')
    return Array.isArray(parsed) ? (parsed as SavedArchiveQuery[]) : []
  } catch {
    return []
  }
}

/** Saves the query under its name; a query with the same name is replaced. */
export function saveQuery(
  name: string,
  query: ArchiveSearchQuery,
  storage: Storage = localStorage,
): SavedArchiveQuery[] {
  const saved = [
    ...loadSavedQueries(storage).filter((entry) => entry.name !== name),
    { name, query: activeFilters(query) },
  ]
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(saved))
  } catch {
    // storage unavailable: the query stays saved for this page view only
  }
  return saved
}

export function deleteSavedQuery(name: string, storage: Storage = localStorage): SavedArchiveQuery[] {
  const saved = loadSavedQueries(storage).filter((entry) => entry.name !== name)
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(saved))
  } catch {
    // storage unavailable
  }
  return saved
}

const CSV_COLUMNS: (keyof ArchivedContract)[] = [
  'did',
  'contract_version',
  'state',
  'name',
  'description',
  'created_by',
  'created_at',
  'updated_at',
  'exp_date',
  'archive_summary',
  'archive_tags',
]

function csvCell(value: ArchivedContract[keyof ArchivedContract]): string {
  if (value === undefined || value === null) return ''
  const text = Array.isArray(value) ? value.join('; ') : String(value)
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}

/** Search results as RFC 4180 CSV for compliance or legal use (DCS-FR-CSA-22). */
export function archiveResultsToCsv(results: ArchivedContract[]): string {
  const rows = results.map((result) => CSV_COLUMNS.map((column) => csvCell(result[column])).join(','))
  return [CSV_COLUMNS.join(','), ...rows].join('\r\n') + '\r\n'
}
