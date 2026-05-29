<script setup lang="ts">
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import { ROUTES } from '@/router/router'
import { toProperCase } from '@/utils/string'

defineProps<{
  template: TemplateResourcesItem
}>()
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ template.name }}</div>
            <div v-if="template.template_type" class="badge badge-accent sm:h-full sm:badge-md">
              {{ toProperCase(template.template_type) }}
            </div>
          </div>
        </h2>
        <div class="flex justify-between">
          <div v-if="template.document_number">Document number: {{ template.document_number }}</div>
          <div v-if="template.version">Version: {{ template.version }}</div>
        </div>
        <div class="flex min-w-0 justify-between">
          <div v-if="template.created_at">Creation date: {{ new Date(template.created_at).toLocaleDateString() }}</div>
          <div v-if="template.description" class="hidden min-w-0 flex-1 truncate px-10 sm:block">
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
