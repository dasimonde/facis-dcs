import axios, { AxiosError } from 'axios'
import { getConfig } from '@/config'
import { authenticationService } from '@/services/authentication-service'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import { useErrorStore } from '@/stores/error-store'
import { bindReportedHttpError } from '@/utils/report-action-error'

const http = axios.create({
  baseURL: getConfig().API_BASE_URL,
  headers: { 'Content-Type': 'application/json' },
  // goa reads a repeated query key as an array (state=A&state=B), not state[]=A.
  paramsSerializer: { indexes: null },
})

http.interceptors.request.use((config) => {
  const tokenStore = useAuthTokenStore()
  config.headers.Authorization = tokenStore.isAuthSet ? tokenStore.getAuthenticationHeader : undefined
  return config
})

http.interceptors.response.use(
  (resp) => resp,
  async (err: Error | AxiosError) => {
    const errorStore = useErrorStore()
    if (axios.isAxiosError(err)) {
      // One retry with a refreshed token; a second 401 is the server's answer.
      const config = err.config as (typeof err.config & { retriedAfterRefresh?: boolean }) | undefined
      // A 401 with Retry-After is a lockout after repeated invalid
      // credentials (FR-UC-01-4): a refreshed token is refused too.
      const lockedFor = Number(err.response?.headers?.['retry-after'])
      if (err.status === 401 && lockedFor > 0) {
        const minutes = Math.ceil(lockedFor / 60)
        err.message = `${err.response?.data?.message ?? 'Access refused.'} Too many invalid credentials came from this address. Try again in ${minutes} minute${minutes === 1 ? '' : 's'}.`
        bindReportedHttpError(err, errorStore.add(err.message))
        return Promise.reject(err)
      }
      if (err.status === 401 && config && !config.retriedAfterRefresh) {
        const isRefreshed = await authenticationService.refresh()
        if (isRefreshed) {
          config.retriedAfterRefresh = true
          return http(config)
        }
      }
    }
    // Callers render the rejected error's own message, so the server's
    // message replaces Axios's "Request failed with status code N".
    if (axios.isAxiosError(err) && typeof err.response?.data?.message === 'string') {
      err.message = err.response.data.message
      // A workflow gate in REVIEW names its run: the Compliance Officer decides it by this ID.
      if (typeof err.response.data.gate_run_id === 'string') {
        err.message += ` Workflow-gate run for the Compliance Officer review: ${err.response.data.gate_run_id}`
      }
    }
    const messageId = errorStore.add(err.message)
    bindReportedHttpError(err, messageId)
    return Promise.reject(err)
  },
)

export default http
