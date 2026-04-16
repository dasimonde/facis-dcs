import { users } from '@/services/user-service'
import { useLocalStorage } from '@vueuse/core'
import { useJwt } from '@vueuse/integrations/useJwt'
import type { JwtPayload as JwtPayloadI } from 'jwt-decode'
import { defineStore } from 'pinia'
import { computed } from 'vue'

interface JwtPayload extends JwtPayloadI {
  preferred_username?: string
}

export const useAuthTokenStore = defineStore('token', () => {
  const tokenType = useLocalStorage<string>('token_type', null)
  const accessToken = useLocalStorage<string>('access_token', null)

  const isAuthSet = computed(() => !!tokenType.value && !!accessToken.value)
  const getAuthenticationHeader = computed(() => `${tokenType.value} ${accessToken.value}`)
  const getUserId = computed(() => {
    setUsername()
    return useJwt(accessToken.value).payload.value?.sub
  })

  function setTokens(type: string, access_token: string) {
    tokenType.value = type
    accessToken.value = access_token
  }

  function remove() {
    tokenType.value = null
    accessToken.value = null
  }

  function setUsername() {
    const username = useJwt<JwtPayload>(accessToken.value).payload.value?.preferred_username
    const userId = useJwt(accessToken.value).payload.value?.sub
    const user = users.value.find((user) => user.username === username)
    if (user && userId) user.id = userId
  }

  return { tokenType, accessToken, isAuthSet, getAuthenticationHeader, getUserId, setTokens, remove }
})
