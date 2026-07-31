<script setup lang="ts">
defineProps<{
  label: string
}>()

defineEmits<{
  select: []
}>()
</script>

<template>
  <div
    class="flex min-h-11 cursor-pointer items-center rounded-lg border border-base-300 bg-base-100 px-3 py-2 transition-colors select-none hover:bg-base-200"
    role="button"
    tabindex="0"
    @click="$emit('select')"
    @keydown.enter.prevent="$emit('select')"
    @keydown.space.prevent="$emit('select')"
  >
    <span class="text-sm font-medium">{{ label }}</span>
  </div>
</template>
