<script setup lang="ts">
import { computed } from 'vue'
import { type RouteLocationRaw, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth-store'

const props = defineProps<{
  steps: { key: string; label: string }[]
  currentKey: string
  headline: string
  narrative: string
  actions?: { label: string; to?: RouteLocationRaw; onClick?: () => void }[]
}>()

const router = useRouter()
const authStore = useAuthStore()

const currentIndex = computed(() => props.steps.findIndex((step) => step.key === props.currentKey))

// The narrative names the role that acts next, which is often not the reader's.
// A CTA whose destination the router guard would bounce is dropped rather than
// rendered: following it lands on the front page with no explanation.
const canReach = (to: RouteLocationRaw): boolean => {
  const resolved = router.resolve(to)
  const roles = resolved.matched.at(-1)?.meta.roles
  if (!roles) return true
  return authStore.user?.roles?.some((role) => roles.includes(role)) ?? false
}

const reachableActions = computed(() => (props.actions ?? []).filter((action) => !action.to || canReach(action.to)))
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
    <div v-if="reachableActions.length" class="mt-3 flex flex-wrap gap-2">
      <template v-for="(action, index) in reachableActions" :key="action.label">
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
