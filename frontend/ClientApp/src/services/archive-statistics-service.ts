import http from '@/api/http'

/** One recent archive-affecting audit event (DCS-FR-CSA-21). */
export interface ArchiveRecentAction {
  actor: string
  occurred_at: string
  event_type: string
  did: string
}

/** An archived contract whose expiration date falls inside the dashboard window. */
export interface ArchiveExpiringContract {
  did: string
  name?: string
  exp_date: string
}

/** The archive dashboard overview the backend aggregates (DCS-FR-CSA-21). */
export interface ArchiveStatistics {
  archived_total: number
  archived_contracts: number
  deleted_total: number
  storage_bytes: number
  compliant_total: number
  flagged_total: number
  recent_actions: ArchiveRecentAction[]
  expiring_contracts: ArchiveExpiringContract[]
}

export const archiveStatisticsService = {
  async statistics(): Promise<ArchiveStatistics> {
    return http.get<ArchiveStatistics>('/archive/statistics').then((res) => res.data)
  },
}
