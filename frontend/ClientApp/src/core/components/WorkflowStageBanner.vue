<script setup lang="ts">
import { computed } from 'vue'
import type { RouteLocationRaw } from 'vue-router'

const props = defineProps<{
  steps: { key: string; label: string }[]
  currentKey: string
  headline: string
  narrative: string
  actions?: { label: string; to?: RouteLocationRaw; onClick?: () => void }[]
}>()

const currentIndex = computed(() => props.steps.findIndex((step) => step.key === props.currentKey))
</script>

<template>
  <div class="rounded-lg border border-base-300 bg-base-200/40 p-4">
    <div class="overflow-x-auto">
      <ul class="steps-xs steps steps-horizontal w-full text-xs">
        <li
          v-for="(step, index) in steps"
          :key="step.key"
          class="step"
          :class="{ 'step-primary': currentIndex >= 0 && index <= currentIndex }"
        >
          {{ step.label }}
        </li>
      </ul>
    </div>
    <p class="mt-3 font-semibold">{{ headline }}</p>
    <p class="mt-1 text-sm text-base-content/70">{{ narrative }}</p>
    <div v-if="actions?.length" class="mt-3 flex flex-wrap gap-2">
      <template v-for="(action, index) in actions" :key="action.label">
        <RouterLink
          v-if="action.to"
          :to="action.to"
          class="btn btn-sm"
          :class="index === 0 ? 'btn-primary' : 'btn-outline'"
        >
          {{ action.label }}
        </RouterLink>
        <button
          v-else
          type="button"
          class="btn btn-sm"
          :class="index === 0 ? 'btn-primary' : 'btn-outline'"
          @click="action.onClick?.()"
        >
          {{ action.label }}
        </button>
      </template>
    </div>
  </div>
</template>
