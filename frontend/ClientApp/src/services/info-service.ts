import http from '@/api/http'
import type { DCSBuildRevision } from '@/config'

export const infoService = {
  async get(): Promise<DCSBuildRevision> {
    return http.get<DCSBuildRevision>('/info').then((res) => res.data)
  },
}
