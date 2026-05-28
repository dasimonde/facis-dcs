<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>

    <RouterLink
      v-if="isTemplateCreator"
      v-slot="{ route }"
      :to="{ name: ROUTES.TEMPLATES.NEW }"
      class="btn gap-2 self-end btn-primary"
    >
      {{ route.meta.name }}
    </RouterLink>
    <div v-else></div>
  </div>
  <div>
    <div v-if="loading" class="pl-4">Loading Templates...</div>
    <div v-else-if="error" class="pl-4">{{ error }}</div>
    <div v-else>
      <TemplateList :templates="templates" :has-review-task="hasReviewTask" :has-approval-task="hasApprovalTask" />
    </div>
  </div>
</template>

<script setup lang="ts">
import TemplateList from '@/components/lists/template/TemplateList.vue'
import { ROUTES } from '@/router/router'
import { computed } from 'vue'
import { useTemplateList } from './ContractTemplateListController'

const { templates, roles, loading, error, hasReviewTask, hasApprovalTask } = useTemplateList()

const isTemplateCreator = computed(() => roles.value.includes('TEMPLATE_CREATOR'))
</script>
