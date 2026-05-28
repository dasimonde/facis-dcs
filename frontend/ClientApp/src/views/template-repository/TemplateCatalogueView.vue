<template>
  <div>
    <div class="mb-8 flex justify-between">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name ?? 'Template Catalogue' }}
      </h2>
    </div>

    <div>
      <div v-if="loading">Lade Templates...</div>
      <div v-else-if="error">{{ error }}</div>
      <div v-else class="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
        <!-- Details -->
        <div class="px-2 pb-6 sm:px-4 lg:col-span-2">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <h3 class="truncate text-lg font-semibold text-base-content">
                {{ catalogue?.name || 'Template details' }}
              </h3>
            </div>
            <div v-if="catalogue?.template_type" class="badge shrink-0 badge-md badge-accent">
              {{ String(catalogue.template_type) }}
            </div>
          </div>

          <div v-if="catalogue" class="mt-4 space-y-5">
            <section class="rounded-xl border border-base-300 bg-base-100">
              <div class="border-b border-base-300 px-4 py-3">
                <div class="text-sm font-semibold">Template</div>
              </div>
              <div class="space-y-2 p-4">
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">DID</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayValue(catalogue.did) }}</div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Document Number</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.document_number) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Version</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayValue(catalogue.version) }}</div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Name</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayValue(catalogue.name) }}</div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Description</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayValue(catalogue.description) }}</div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Template Type</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.template_type) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Created At</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayDate(catalogue.created_at) }}</div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Updated At</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">{{ displayDate(catalogue.updated_at) }}</div>
                </div>
              </div>
            </section>

            <section v-if="catalogue.participant" class="rounded-xl border border-base-300 bg-base-100">
              <div class="border-b border-base-300 px-4 py-3">
                <div class="text-sm font-semibold">Participant</div>
              </div>
              <div class="space-y-2 p-4">
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Legal Name</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.participant.legal_name) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Registration Number</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.participant.registration_number) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">LEI Code</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.participant.lei_code) }}
                  </div>
                </div>
                <div
                  v-if="catalogue.participant.ethereum_address"
                  class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4"
                >
                  <div class="text-xs font-semibold text-base-content/60">Ethereum Address</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.participant.ethereum_address) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Terms and Conditions</div>
                  <div class="font-mono text-sm break-all sm:col-span-2">
                    {{ displayValue(catalogue.participant.terms_and_conditions) }}
                  </div>
                </div>

                <div v-if="catalogue.participant.headquarter_address" class="pt-2">
                  <div class="mb-2 text-xs font-semibold text-base-content/60">Headquarter Address</div>
                  <div class="space-y-2 rounded-lg p-3">
                    <div
                      v-if="catalogue.participant.headquarter_address?.country"
                      class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4"
                    >
                      <div class="text-xs font-semibold text-base-content/60">Country</div>
                      <div class="font-mono text-sm break-all sm:col-span-2">
                        {{ displayValue(catalogue.participant.headquarter_address?.country) }}
                      </div>
                    </div>
                    <div
                      v-if="catalogue.participant.headquarter_address?.locality"
                      class="grid grid-cols-1 gap-1 sm:grid-cols-3 sm:gap-4"
                    >
                      <div class="text-xs font-semibold text-base-content/60">Locality</div>
                      <div class="font-mono text-sm break-all sm:col-span-2">
                        {{ displayValue(catalogue.participant.headquarter_address?.locality) }}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </section>
          </div>
          <p v-else class="text-sm text-base-content/70">No data loaded.</p>
        </div>

        <!-- Actions -->
        <div class="lg:pr-6">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body">
              <p class="text-sm text-base-content/70">This template is free to use.</p>
              <div class="flex flex-col gap-2 pt-2">
                <RouterLink
                  class="btn rounded-box btn-sm btn-primary"
                  :to="{ name: 'ROUTES.TEMPLATE_CATALOGUES.NEGOTIATION_CREATE', params: { did } }"
                >
                  Create Negotiation
                </RouterLink>
                <button class="btn rounded-box btn-outline btn-sm" @click="router.back()">Back</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import type { TemplateResource } from '@/modules/template-catalogue/models/template-resource'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const error = ref<string | null>(null)
const catalogue = ref<TemplateResource | null>(null)

const did = computed(() => String(route.params.did ?? ''))

async function load() {
  loading.value = true
  error.value = null
  try {
    catalogue.value = await templateCatalogueIntegrationService.retrieve_template_by_id({ did: did.value })
  } catch (e: unknown) {
    error.value = e instanceof Error && e.message ? e?.message : 'Error loading template catalogue'
  } finally {
    loading.value = false
  }
}

void load()

/* eslint-disable @typescript-eslint/no-base-to-string */
function displayValue(value: unknown): string {
  return value === null || value === undefined || value === '' ? '' : String(value)
}

function displayDate(value: unknown): string {
  if (value === null || value === undefined || value === '') return ''
  const d = new Date(String(value))
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleDateString()
}
/* eslint-enable @typescript-eslint/no-base-to-string */
</script>
