<script setup lang="ts">
import { computed } from 'vue'
import { PREVIEW_SECTION_CHILDREN_CLASS } from './preview-classes'
const props = defineProps<{
  title: string
  hasChildren?: boolean
  level?: number
}>()

const sectionChildrenClass = PREVIEW_SECTION_CHILDREN_CLASS

const headingClass = computed(() => {
  const level = Math.max(1, props.level ?? 1)
  if (level <= 1) return 'text-lg' // level 1: top-level section
  if (level === 2) return 'text-base' // level 2
  if (level === 3) return 'text-sm' // level 3
  if (level === 4) return 'text-xs' // level 4
  return 'text-[0.7rem]' // level 5 and deeper
})
</script>

<template>
  <h2 class="mb-1 border-b border-base-300 pb-1 font-bold text-base-content" :class="headingClass">
    {{ title }}
  </h2>
  <div v-if="hasChildren" :class="sectionChildrenClass">
    <slot />
  </div>
</template>

<style scoped>
.section-children > section {
  width: 100%;
  flex-basis: 100%;
}
</style>
