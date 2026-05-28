<script setup lang="ts">
withDefaults(
  defineProps<{
    /** When true, wrap default slot in an element; when false, render slot content without a wrapper. */
    enabled: boolean
    /** HTML element tag for the wrapper when enabled (e.g. 'div', 'section'). */
    tag?: string
    /** Class(es) applied to the wrapper when enabled. Supports string, array, or object (same as Vue :class). */
    wrapperClass?: string | string[] | Record<string, boolean | undefined>
  }>(),
  { tag: 'div' },
)
</script>

<template>
  <component :is="tag" v-if="enabled" :class="wrapperClass">
    <slot />
  </component>
  <template v-else>
    <slot />
  </template>
</template>
