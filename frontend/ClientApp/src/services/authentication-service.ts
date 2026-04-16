import authHttp from '@/api/auth-http'
import type { AuthCallbackResponse, LoginResponse, LogoutResponse } from '@/models/responses/auth-response'
import type { AuthenticationService } from '@/models/services/authentication-service'
import { useAuthStore } from '@/stores/auth-store'
import { useAuthTokenStore } from '@/stores/auth-token-store'

export const authenticationService: AuthenticationService = {
  async loginPath() {
    return await authHttp
      .get<LoginResponse>('/auth/login')
      .then((res) => res.data.auth_url)
      .catch((err) => {
        console.error('Login Error:', err)
        return ''
      })
  },

  async refresh() {
    return authHttp
      .post<AuthCallbackResponse>('/auth/refresh')
      .then((res) => {
        const authTokenStore = useAuthTokenStore()
        authTokenStore.setTokens(res.data.token_type, res.data.access_token)
        const authStore = useAuthStore()
        const userId = authTokenStore.getUserId
        if (!userId) throw new Error('JWT Error')
        authStore.setUser(userId)
        return true
      })
      .catch((err) => {
        if (err && err.status === 401) {
          const authStore = useAuthStore()
          authStore.remove()
          const authTokenStore = useAuthTokenStore()
          authTokenStore.remove()
        }
        return false
      })
  },

  logout() {
    // Clear local state first
    const authStore = useAuthStore()
    authStore.remove()
    const authTokenStore = useAuthTokenStore()
    authTokenStore.remove()

    // Call backend logout endpoint to get Keycloak logout URL (mirrors login flow)
    authHttp
      .get<LogoutResponse>('/auth/logout')
      .then((res) => {
        window.location.href = res.data.logout_url
      })
      .catch((err) => {
        console.error('Logout Error:', err)
        // Fallback to home if logout fails
        window.location.href = '/'
      })
  },
}
