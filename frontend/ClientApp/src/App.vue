<script setup lang="ts">
import { PageLayout } from '@core/layouts/page'
import { useSyncPageTitle } from '@core/composables/useSyncPageTitle'
import { RouterView, useRoute } from 'vue-router'
import { computed } from 'vue'
import ErrorToast from './components/ErrorToast.vue'

useSyncPageTitle()

const route = useRoute()
const isBlankLayout = computed(() => route.meta.layout === 'blank')
</script>

<template>
  <RouterView v-if="isBlankLayout" />
  <PageLayout v-else />
  <ErrorToast />
</template>
