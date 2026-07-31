import http from '@/api/http'

/** One HSM-held key with its purpose and active version (read-only; rotation is an operator procedure). */
export interface HSMKeyInfo {
  label: string
  purpose: string
  active_version: number
  updated_at?: string
}

export const keyInventoryService = {
  async list(): Promise<HSMKeyInfo[]> {
    return http.get<{ keys: HSMKeyInfo[] }>('/admin/hsm-keys').then((res) => res.data.keys)
  },
}
