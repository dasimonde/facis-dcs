<script setup lang="ts">
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import { ROUTES } from '@/router/router'
import { toProperCase } from '@/utils/string'

const props = defineProps<{
  template: TemplateResourcesItem
}>()
</script>

<template>
  <li class="list-row min-w-0 w-full">
    <div class="list-col-grow card bg-base-100 card-border hover:bg-base-300 min-w-0 w-full border-base-content/10">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ template.name }}</div>
            <div v-if="template.template_type" class="badge sm:badge-md badge-accent sm:h-full">
              {{ toProperCase(template.template_type) }}
            </div>
          </div>
        </h2>
        <div class="flex justify-between">
          <div v-if="template.document_number">Document number: {{ template.document_number }}</div>
          <div v-if="template.version">Version: {{ template.version }}</div>
        </div>
        <div class="flex justify-between min-w-0">
          <div v-if="template.created_at">Creation date: {{ new Date(template.created_at).toLocaleDateString() }}</div>
          <div v-if="template.description" class="px-10 flex-1 min-w-0 truncate hidden sm:block">
            {{ template.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="{
                name: ROUTES.TEMPLATE_CATALOGUES.VIEW,
                params: { did: template.did },
                query: {
                  version: template.version,
                },
              }"
              class="btn btn-sm btn-primary"
            >
              View
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </li>
</template>

