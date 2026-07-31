<script setup lang="ts">
import { ChevronLeftIcon, ChevronRightIcon } from '@heroicons/vue/20/solid'

defineOptions({
  name: 'AppPagination',
})

defineProps<{
  currentPage: number
  hasNextPage: boolean
}>()

defineEmits<{
  previousPage: []
  nextPage: []
}>()
</script>

<template>
  <div class="flex w-full justify-center gap-2">
    <button
      type="button"
      class="btn px-4 btn-outline btn-sm"
      :disabled="currentPage === 1"
      aria-label="Go to previous page"
      @click="$emit('previousPage')"
    >
      <ChevronLeftIcon class="size-5" />
    </button>
    <button
      type="button"
      class="btn btn-active w-13.5 px-4 btn-outline btn-sm btn-primary"
      aria-current="page"
      :aria-label="`Current page, page ${currentPage}`"
    >
      {{ currentPage }}
    </button>
    <button
      type="button"
      class="btn px-4 btn-outline btn-sm"
      :disabled="!hasNextPage"
      aria-label="Go to next page"
      @click="$emit('nextPage')"
    >
      <ChevronRightIcon class="size-5" />
    </button>
  </div>
</template>
