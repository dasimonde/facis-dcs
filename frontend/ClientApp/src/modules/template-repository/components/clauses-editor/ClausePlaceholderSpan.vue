<template>
  <span
    class="clause-placeholder-slot inline-block cursor-default rounded-sm border-b-2 border-dashed border-primary/50 bg-primary/5 px-1 font-medium text-primary"
  >
    {{ label }}
  </span>
</template>

<script setup lang="ts">
defineProps<{
  label: string
}>()
</script>
