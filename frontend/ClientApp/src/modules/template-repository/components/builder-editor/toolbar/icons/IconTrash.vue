<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 20 20"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    :class="$attrs.class as string"
  >
    <path d="M5 6h10v10a1 1 0 01-1 1H6a1 1 0 01-1-1V6z" />
    <path d="M4 6h12M8 6V5a1 1 0 011-1h2a1 1 0 011 1v1M10 10v4M8 10v4M12 10v4" />
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
