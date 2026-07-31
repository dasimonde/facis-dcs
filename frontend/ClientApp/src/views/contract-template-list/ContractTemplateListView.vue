<script setup lang="ts">
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import TemplateList from '@/components/lists/template/TemplateList.vue'
import { ROUTES } from '@/router/router'

const { isCreator } = useTemplatePermissions()
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="mb-4 flex shrink-0 justify-between border-b border-base-content/10 bg-base-100 p-4">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name }}
      </h2>

      <RouterLink
        v-if="isCreator"
        v-slot="{ route }"
        :to="{ name: ROUTES.TEMPLATES.NEW }"
        class="btn gap-2 self-end btn-primary"
      >
        {{ route.meta.name }}
      </RouterLink>
      <div v-else></div>
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      <TemplateList />
    </div>
  </div>
</template>
