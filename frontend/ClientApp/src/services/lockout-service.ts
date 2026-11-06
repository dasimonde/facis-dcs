import http from '@/api/http'

/** One active lockout after repeated invalid credentials (FR-UC-01-4). */
export interface LockoutInfo {
  /** A client address, or holder:<did:jwk> for a credential holder key. */
  key: string
  locked_until: string
}

export const lockoutService = {
  async list(): Promise<LockoutInfo[]> {
    return http.get<LockoutInfo[]>('/admin/lockouts').then((res) => res.data)
  },
  async unlock(key: string): Promise<void> {
    await http.post('/admin/lockouts/unlock', { key })
  },
}
