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
    <path d="M6 10h8M10 6l4 4-4 4" />
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
