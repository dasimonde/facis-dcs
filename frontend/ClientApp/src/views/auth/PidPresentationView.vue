<script setup lang="ts">
import { ref } from 'vue'
import PidQrPresentation from '@/components/auth/PidQrPresentation.vue'

const verified = ref(false)
const errorMessage = ref('')
const presentationKey = ref(0)

function onSuccess() {
  verified.value = true
  errorMessage.value = ''
}

function onFailed(message: string) {
  verified.value = false
  errorMessage.value = message
}

function verifyAgain() {
  verified.value = false
  errorMessage.value = ''
  presentationKey.value++
}
</script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center gap-6 bg-base-200 p-6">
    <div v-if="verified" class="card w-full max-w-md bg-base-100 shadow-md">
      <div class="card-body items-center gap-3 text-center">
        <h1 class="card-title text-lg text-success">PID verified</h1>
        <p class="text-sm opacity-80">Your PID credential was presented and verified successfully.</p>
        <button type="button" class="btn btn-sm btn-primary" @click="verifyAgain">Verify again</button>
      </div>
    </div>

    <div v-else-if="errorMessage" class="card w-full max-w-md bg-base-100 shadow-md">
      <div class="card-body items-center gap-3 text-center">
        <h1 class="card-title text-lg text-error">Verification failed</h1>
        <p class="text-sm opacity-80">{{ errorMessage }}</p>
        <button type="button" class="btn btn-sm btn-primary" @click="verifyAgain">Try again</button>
      </div>
    </div>

    <PidQrPresentation v-else :key="presentationKey" @success="onSuccess" @failed="onFailed" />
  </div>
</template>
