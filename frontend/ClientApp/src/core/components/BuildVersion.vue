<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { infoService } from '@/services/info-service'
import { logDcsVersion } from '@/utils/log-dcs-version'
import type { DCSVersionInfo } from '@/config'

const version = ref<DCSVersionInfo | undefined>(window.__DCS_VERSION__)

const short = (sha?: string) => (sha ?? 'unknown').slice(0, 7)

onMounted(async () => {
  try {
    const backend = await infoService.get()
    window.__DCS_VERSION__ ??= { frontend: { git_sha: 'unknown' } }
    window.__DCS_VERSION__.backend = backend
  } catch {}
  if (window.__DCS_VERSION__) {
    version.value = { ...window.__DCS_VERSION__ }
    logDcsVersion(window.__DCS_VERSION__)
  }
})
</script>

<template>
  <div class="px-4 py-3 font-mono text-xs text-base-content/70" data-testid="build-version">
    <div :title="version?.frontend.git_sha">Frontend {{ short(version?.frontend.git_sha) }}</div>
    <div :title="version?.backend?.git_sha">Backend {{ short(version?.backend?.git_sha) }}</div>
  </div>
</template>
