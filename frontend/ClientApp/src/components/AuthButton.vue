<script setup lang="ts">
import { authenticationService } from '@/services/authentication-service'
import { useAuthStore } from '@/stores/auth-store'
import { ArrowLeftStartOnRectangleIcon } from '@heroicons/vue/24/outline'
import { storeToRefs } from 'pinia'

const authStore = useAuthStore()
const { isAuthenticated } = storeToRefs(authStore)

const logout = () => authenticationService.logout()
</script>

<template>
  <div>
    <button v-if="isAuthenticated" class="btn gap-2 btn-outline btn-sm" @click="logout">
      <ArrowLeftStartOnRectangleIcon class="size-4" />
      Log out
    </button>
  </div>
</template>
