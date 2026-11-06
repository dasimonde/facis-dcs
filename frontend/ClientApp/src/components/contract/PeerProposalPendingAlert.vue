<script setup lang="ts">
defineProps<{
  show: boolean
  message: string
}>()
</script>

<template>
  <div v-if="show" class="alert text-sm alert-warning" role="status" data-testid="peer-proposal-pending-alert">
    {{ message }}
  </div>
</template>
