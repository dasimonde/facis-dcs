<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { authenticationService } from '@/services/authentication-service'
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

onMounted(async () => {
  // Exchange refresh token cookie for access token
  const result = await authenticationService.refresh()
  // Redirect to templates list on success
  if (result) {
    await router.replace({ name: ROUTES.TEMPLATES.LIST })
  } else {
    await router.replace({ name: ROUTES.HOME })
  }
})
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-base-200">
    <span class="loading loading-lg loading-spinner" />
  </div>
</template>
