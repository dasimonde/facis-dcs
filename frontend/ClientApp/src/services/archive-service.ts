import http from '@/api/http'

/** An archived contract entry as returned by /archive/retrieve. */
export interface ArchivedContract {
  did: string
  contract_version: number
  state: string
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  exp_date?: string
  archive_summary?: string
  archive_tags?: string[]
}

/** The erasure-handshake state of one counterparty instance (DCS-NFR-COMP-03). */
export interface ArchiveErasurePeerStatus {
  peer_did: string
  status: 'pending' | 'confirmed'
  requested_at: string
  confirmed_at?: string
  retry_count: number
  last_tried_at?: string
}

/** The erasure state of a contract's content-encryption keys (DCS-NFR-SEC-13). */
export interface ArchiveErasureStatus {
  did: string
  local_status: 'live' | 'shredded'
  shredded_at?: string
  shredded_by?: string
  shred_reason?: string
  peers: ArchiveErasurePeerStatus[]
}

export const archiveService = {
  async retrieve(): Promise<ArchivedContract[]> {
    return http.get<{ contracts?: ArchivedContract[] }>('/archive/retrieve').then((res) => res.data.contracts ?? [])
  },

  async erasureStatus(did: string): Promise<ArchiveErasureStatus> {
    return http.get<ArchiveErasureStatus>('/archive/erasure-status', { params: { did } }).then((res) => res.data)
  },

  /**
   * Soft-deletes the archive entry and destroys the contract's content
   * encryption keys on both instances; the justification is recorded as the
   * shred reason (DCS-FR-CSA-17, DCS-NFR-SEC-13).
   */
  async delete(did: string, justification: string): Promise<number> {
    return http.delete<number>('/archive/delete', { params: { did, justification } }).then((res) => res.data)
  },
}
