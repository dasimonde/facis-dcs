import axios, { AxiosError } from 'axios'
import { getConfig } from '@/config'
import { authenticationService } from '@/services/authentication-service'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import { useErrorStore } from '@/stores/error-store'
import { bindReportedHttpError } from '@/utils/report-action-error'

const http = axios.create({
  baseURL: getConfig().API_BASE_URL,
  headers: { 'Content-Type': 'application/json' },
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
      if (err.status === 401 && err.config) {
        const isRefreshed = await authenticationService.refresh()
        if (isRefreshed) {
          return http(err.config)
        }
      }
    }
    // Callers render the rejected error's own message, so the server's
    // message replaces Axios's "Request failed with status code N".
    if (axios.isAxiosError(err) && typeof err.response?.data?.message === 'string') {
      err.message = err.response.data.message
    }
    const messageId = errorStore.add(err.message)
    bindReportedHttpError(err, messageId)
    return Promise.reject(err)
  },
)

export default http
