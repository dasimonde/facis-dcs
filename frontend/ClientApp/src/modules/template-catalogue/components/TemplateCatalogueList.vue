<script setup lang="ts">
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import TemplateCatalogueListItem from './TemplateCatalogueListItem.vue'

defineProps<{
  templates: TemplateResourcesItem[]
}>()
</script>

<template>
  <ul class="list">
    <TemplateCatalogueListItem
      v-for="template in templates"
      :key="`${template.did}|${template.document_number}|${template.version}`"
      :template="template"
    />
    <li v-if="templates.length < 1" class="px-4">No templates found</li>
  </ul>
</template>
