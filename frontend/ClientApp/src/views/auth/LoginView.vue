<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { authenticationService } from '@/services/authentication-service'
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

onMounted(async () => {
  // Keycloak kann je nach Konfiguration auch auf '/' zurückleiten.
  // In dem Fall direkt zu auth.success forwarden, ohne beforeEach zu involvieren.
  if (route.query.session_state && route.query.code && route.query.iss) {
    router.replace({ name: ROUTES.AUTH.SUCCESS, query: route.query })
    return
  }

  const loginUrl = await authenticationService.loginPath()
  if (loginUrl) {
    window.location.href = loginUrl
  }
})
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-base-200">
    <span class="loading loading-spinner loading-lg" />
  </div>
</template>
