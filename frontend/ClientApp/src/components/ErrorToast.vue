<script setup lang="ts">
import { useErrorStore } from '@/stores/error-store'
import { storeToRefs } from 'pinia'

const errorStore = useErrorStore()
const { errors } = storeToRefs(errorStore)
</script>

<template>
  <div class="toast">
    <div
      v-for="error in errors"
      :key="error.id"
      :class="['alert', `alert-${error.type}`, 'max-h-40 max-w-md overflow-y-auto text-wrap shadow-lg']"
      :role="error.type === 'error' ? 'alert' : 'status'"
    >
      <span>{{ error.message }}</span>
    </div>
  </div>
</template>
