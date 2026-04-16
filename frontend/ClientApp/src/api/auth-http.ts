import { getConfig } from '@/config'
import { useErrorStore } from '@/stores/error-store'
import axios from 'axios'

const http = axios.create({
  baseURL: getConfig().API_BASE_URL,
  headers: { 'Content-Type': 'application/json' },
})

http.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const errorStore = useErrorStore()
    const message = axios.isAxiosError(err) ? err.response?.data.message || err.message : err.message
    errorStore.add(String(message))
    return Promise.reject(err)
  },
)

export default http
