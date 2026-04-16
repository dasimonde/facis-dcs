<script setup lang="ts">
import { ref, watch } from 'vue'

const emit = defineEmits<{
  pageChange: [value: number]
}>()

defineProps<{
  pages: number
}>()

const currentPage = ref(1)

watch(currentPage, (newPage, oldPage) => {
  if (newPage !== oldPage) {
    emit('pageChange', currentPage.value)
  }
})
</script>

<template>
  <div v-if="pages > 0" class="join w-full justify-center">
    <template v-for="page in pages" :key="page">
      <button
        type="button"
        class="btn join-item btn-outline btn-accent"
        :class="{ 'btn-active': page === currentPage }"
        @click="currentPage = page"
      >
        {{ page }}
      </button>
    </template>
  </div>
</template>
