<template>
  <div>
    <div class="mb-4 flex justify-between p-4">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name ?? 'Template Catalogues' }}
      </h2>
    </div>

    <div>
      <div v-if="loading">Lade Templates...</div>
      <div v-else-if="error">{{ error }}</div>
      <div v-else>
        <ul class="list">
          <li v-if="!items.length" class="py-6 text-center text-base-content/60">No data found.</li>
          <!-- card -->
          <li v-for="item in items" :key="item.did" class="list-row w-full min-w-0">
            <div class="list-col-grow card w-full min-w-0 bg-base-200 card-border hover:bg-base-300">
              <div class="card-body min-w-0">
                <h2 class="card-title flex-wrap sm:justify-between">
                  <div class="flex gap-8 sm:h-full">
                    <div>Name: {{ item.name }}</div>
                    <div v-if="item.template_type" class="badge badge-accent sm:h-full sm:badge-md">
                      {{ toProperCase(item.template_type) }}
                    </div>
                  </div>
                </h2>
                <div class="flex flex-col justify-between gap-2">
                  <div v-if="item.document_number">Document number: {{ item.document_number }}</div>
                  <div v-if="item.version">Version: {{ item.version }}</div>
                </div>
                <div class="flex min-w-0 justify-between">
                  <div v-if="item.created_at">Creation date: {{ new Date(item.created_at).toLocaleDateString() }}</div>
                  <div v-if="item.description" class="hidden min-w-0 flex-1 truncate px-10 sm:block">
                    {{ item.description }}
                  </div>
                  <div class="card-actions flex-none justify-end">
                    <RouterLink
                      :to="{ name: 'template.catalogues.view', params: { did: item.did } }"
                      class="btn btn-sm btn-primary"
                    >
                      View
                    </RouterLink>
                  </div>
                </div>
              </div>
            </div>
          </li>
        </ul>

        <div v-if="totalPages > 1" class="mt-4 flex items-center justify-end gap-3">
          <span class="text-xs text-base-content/70">Page {{ page + 1 }} of {{ totalPages }}</span>
          <div class="join">
            <button class="btn join-item btn-xs" :disabled="!canPrev" @click="goPrev">Prev</button>
            <button class="btn join-item btn-xs" :disabled="!canNext" @click="goNext">Next</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'
import { toProperCase } from '@/utils/string'
import { computed, ref } from 'vue'
const pageSize = 20
const page = ref(0)
const items = ref<TemplateResourcesItem[]>([])

const loading = ref(false)
const error = ref<string | null>(null)
const totalCount = ref(0)

async function load() {
  loading.value = true
  error.value = null
  try {
    const offset = page.value * pageSize
    const resp = await templateCatalogueIntegrationService.retrieve_template({ offset, limit: pageSize })
    items.value = resp.items
    totalCount.value = resp.totalCount
  } catch (e: unknown) {
    error.value = e instanceof Error && e.message ? e?.message : 'Error loading template catalogues'
  } finally {
    loading.value = false
  }
}
void load()

const totalPages = computed(() => {
  if (loading.value) return 0
  return totalCount.value > 0 ? Math.ceil(totalCount.value / pageSize) : 1
})

const canPrev = computed(() => page.value > 0)
const canNext = computed(() => (page.value + 1) * pageSize < totalCount.value)

function goPrev() {
  if (!canPrev.value) return
  page.value -= 1
  void load()
}

function goNext() {
  if (!canNext.value) return
  page.value += 1
  void load()
}
</script>
