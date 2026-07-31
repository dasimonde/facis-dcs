<script setup lang="ts">
defineProps<{
  label: string
}>()
</script>

<template>
  <span
    class="clause-placeholder-slot inline-block cursor-default rounded-sm border-0 border-b border-neutral/70 bg-primary/5 px-1.5 py-0.5 font-medium text-primary"
  >
    {{ label }}
  </span>
</template>
