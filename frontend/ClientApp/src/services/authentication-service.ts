import axios from 'axios'
import authHttp from '@/api/auth-http'
import { clearOid4vpBrowserSession } from '@/hydra-login-guard'
import {
  type AuthCallbackResponse,
  LOGIN_POLL_ERROR,
  type LoginPollStatus,
  type LoginResponse,
  type LoginStatusResponse,
  type LogoutResponse,
} from '@/models/responses/auth-response'
import { useAuthStore } from '@/stores/auth-store'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import type { AuthenticationService } from '@/models/services/authentication-service'

export const authenticationService: AuthenticationService = {
  async login() {
    return authHttp
      .post<LoginResponse>('/auth/login')
      .then((res) => ({
        presentationUrl: res.data.presentation_url,
        state: res.data.state,
        authorizeUrl: res.data.authorize_url,
        expiresIn: res.data.expires_in,
      }))
      .catch((err: unknown) => {
        console.error('Login initiate error:', err)
        return null
      })
  },

  async loginRenew(state: string) {
    return authHttp
      .post<LoginResponse>('/auth/login/renew', { state })
      .then((res) => ({
        presentationUrl: res.data.presentation_url,
        state: res.data.state,
        authorizeUrl: res.data.authorize_url,
        expiresIn: res.data.expires_in,
      }))
      .catch((err: unknown) => {
        console.error('Login renew error:', err)
        return null
      })
  },

  async loginPollStatus(state: string): Promise<LoginPollStatus> {
    try {
      const res = await authHttp.get<LoginStatusResponse>('/auth/login/status', {
        params: { state },
        timeout: 30_000,
      })
      return res.data
    } catch (err: unknown) {
      if (axios.isAxiosError(err) && err.code === 'ECONNABORTED') {
        return LOGIN_POLL_ERROR.TIMEOUT
      }
      if (axios.isAxiosError(err) && err.response?.status === 404) {
        return LOGIN_POLL_ERROR.NOT_FOUND
      }
      console.error('Login status error:', err)
      return null
    }
  },

  async refresh() {
    return authHttp
      .post<AuthCallbackResponse>('/auth/refresh')
      .then((res) => {
        const authTokenStore = useAuthTokenStore()
        authTokenStore.setTokens(res.data.token_type, res.data.access_token)
        const authStore = useAuthStore()
        const holder = authTokenStore.getHolder
        if (!holder) throw new Error('JWT Error')
        authStore.setHolder(holder)
        return authStore.isAuthenticated
      })
      .catch((err: unknown) => {
        if (axios.isAxiosError(err) && err.response?.status === 401) {
          const authStore = useAuthStore()
          authStore.remove()
          const authTokenStore = useAuthTokenStore()
          authTokenStore.remove()
        }
        return false
      })
  },

  logout() {
    const clearLocalAuth = () => {
      useAuthStore().remove()
      useAuthTokenStore().remove()
      clearOid4vpBrowserSession()
    }

    const doLogout = (retried: boolean) => {
      authHttp
        .get<LogoutResponse>('/auth/logout')
        .then((res) => {
          clearLocalAuth()
          // Hydra end-session URL must include id_token_hint which user/session to log out.
          if (!res.data.logout_url?.includes('id_token_hint=')) {
            console.error('Logout: missing id_token_hint in logout_url')
            return
          }
          window.location.href = res.data.logout_url
        })
        .catch(async (err: unknown) => {
          console.error('Logout Error:', err)
          if (!retried) {
            const refreshed = await authenticationService.refresh()
            if (refreshed) {
              doLogout(true)
              return
            }
          }
          clearLocalAuth()
        })
    }

    doLogout(false)
  },
}
