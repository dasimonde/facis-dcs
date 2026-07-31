import { useJwt } from '@vueuse/integrations/useJwt'
import { defineStore } from 'pinia'
import { computed, type Ref, ref } from 'vue'
import { mapRoleLabelsToUserRoles, rolesFromJwtPayload, type UserRole } from '@/types/user-role'
import { useAuthTokenStore } from './auth-token-store'

interface User {
  issuer: string
  holder: string
  roles: UserRole[]
}

export const useAuthStore = defineStore('auth', () => {
  const authTokenStore = useAuthTokenStore()
  const user: Ref<User | null> = ref(null)

  const isAuthenticated = computed(() => !!user.value && authTokenStore.isAuthSet)

  function setHolder(holder: string) {
    const authTokenStore = useAuthTokenStore()
    const payload = useJwt<{
      sub?: string
      roles?: unknown
      ext?: { iss?: string; roles?: unknown }
    }>(authTokenStore.accessToken).payload.value

    if (payload?.sub !== holder) {
      console.error('User Error: JWT sub mismatch', { expected: holder, sub: payload?.sub })
      return
    }

    const roles = mapRoleLabelsToUserRoles(rolesFromJwtPayload(payload))
    if (roles.length === 0) {
      console.error('User Error: Hydra access token has no mapped roles', { sub: payload.sub })
      return
    }

    user.value = {
      holder: holder,
      issuer: payload?.ext?.iss ?? '',
      roles,
    }
  }

  /**
   * Rehydrate the user from the stored access token when it is present and
   * unexpired — the token already carries sub, roles, and issuer, so a page
   * reload does not need a /auth/refresh round-trip (which spends a
   * single-use refresh token) just to re-establish identity. Returns whether
   * a valid session was restored.
   */
  function restoreFromToken(): boolean {
    const payload = useJwt<{
      sub?: string
      exp?: number
      roles?: unknown
      ext?: { iss?: string; roles?: unknown }
    }>(authTokenStore.accessToken).payload.value

    if (!payload?.sub || (payload.exp && payload.exp * 1000 <= Date.now())) {
      return false
    }
    const roles = mapRoleLabelsToUserRoles(rolesFromJwtPayload(payload))
    if (roles.length === 0) {
      return false
    }
    user.value = { holder: payload.sub, issuer: payload.ext?.iss ?? '', roles }
    return true
  }

  function remove() {
    user.value = null
  }

  return { user, isAuthenticated, setHolder, restoreFromToken, remove }
})
