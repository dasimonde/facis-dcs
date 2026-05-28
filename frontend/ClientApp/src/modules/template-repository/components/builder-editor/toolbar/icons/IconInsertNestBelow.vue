<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 20 20"
    fill="none"
    stroke="currentColor"
    stroke-width="1.1"
    stroke-linecap="round"
    stroke-linejoin="round"
    :class="$attrs.class as string"
  >
    <rect x="1.5" y="1.5" width="17" height="6" rx="0.6" />
    <rect x="8.5" y="9.5" width="10" height="6" rx="0.6" stroke-dasharray="2 1.5" />
    <path d="M13.5 11.3v2.4M12.1 12.5h2.8" stroke="currentColor" stroke-width="1.1" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: number
  }>(),
  { size: 20 },
)
</script>
