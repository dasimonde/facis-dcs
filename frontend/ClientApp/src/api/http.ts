import { getConfig } from '@/config'
import { authenticationService } from '@/services/authentication-service'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import { useErrorStore } from '@/stores/error-store'
import axios, { AxiosError } from 'axios'

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
    const message = axios.isAxiosError(err) ? err.response?.data?.message || err.message : err.message
    errorStore.add(String(message))
    return Promise.reject(err)
  },
)

export default http
