<script setup lang="ts">
import { onMounted } from 'vue'
import TemplateCatalogueList from '@template-catalogue/components/catalogue-template/TemplateCatalogueList.vue'
import { useTemplateCatalogueList } from '@template-catalogue/composables/useTemplateCatalogueList'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'

const { templates, loading, error, refresh } = useTemplateCatalogueList()
const templatesStore = useContractTemplatesStore()

onMounted(() => {
  void refresh()
  void templatesStore.loadTemplates()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="mb-4 flex shrink-0 justify-between border-b border-base-content/10 bg-base-100 p-4">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name }}
      </h2>
      <div />
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      <div v-if="loading" class="pl-4">Loading Templates...</div>
      <div v-else-if="error" class="pl-4">{{ error }}</div>
      <div v-else class="h-full">
        <TemplateCatalogueList :templates="templates" />
      </div>
    </div>
  </div>
</template>
