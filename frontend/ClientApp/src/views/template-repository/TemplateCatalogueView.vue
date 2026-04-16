<template>
  <div>
    <div class="flex justify-between mb-8">
      <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
        {{ $route.meta.name ?? 'Template Catalogue' }}
      </h2>
    </div>

    <div>
      <div v-if="loading">Lade Templates...</div>
      <div v-else-if="error">{{ error }}</div>
      <div v-else class="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        <!-- Details -->
        <div class="lg:col-span-2 px-2 sm:px-4 pb-6">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <h3 class="text-lg font-semibold text-base-content truncate">
                {{ catalogue?.name || 'Template details' }}
              </h3>
            </div>
            <div v-if="catalogue?.template_type" class="badge badge-md badge-accent shrink-0">
              {{ String(catalogue.template_type) }}
            </div>
          </div>

          <div v-if="catalogue" class="space-y-5 mt-4">
            <section class="rounded-xl border border-base-300 bg-base-100">
              <div class="px-4 py-3 border-b border-base-300">
                <div class="font-semibold text-sm">Template</div>
              </div>
              <div class="p-4 space-y-2">
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">DID</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.did) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Document Number</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.document_number) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Version</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.version) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Name</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.name) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Description</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.description) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Template Type</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.template_type) }}
                  </div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Created At</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayDate(catalogue.created_at) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Updated At</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayDate(catalogue.updated_at) }}</div>
                </div>
              </div>
            </section>

            <section v-if="catalogue.participant" class="rounded-xl border border-base-300 bg-base-100">
              <div class="px-4 py-3 border-b border-base-300">
                <div class="font-semibold text-sm">Participant</div>
              </div>
              <div class="p-4 space-y-2">
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Legal Name</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{
                    displayValue(catalogue.participant.legal_name) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Registration Number</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{
                    displayValue(catalogue.participant.registration_number) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">LEI Code</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{ displayValue(catalogue.participant.lei_code)
                  }}</div>
                </div>
                <div v-if="catalogue.participant.ethereum_address"
                  class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Ethereum Address</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{
                    displayValue(catalogue.participant.ethereum_address) }}</div>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                  <div class="text-xs font-semibold text-base-content/60">Terms and Conditions</div>
                  <div class="sm:col-span-2 text-sm font-mono break-all">{{
                    displayValue(catalogue.participant.terms_and_conditions) }}</div>
                </div>

                <div v-if="catalogue.participant.headquarter_address" class="pt-2">
                  <div class="text-xs font-semibold text-base-content/60 mb-2">Headquarter Address</div>
                  <div class="rounded-lg p-3 space-y-2">
                    <div v-if="catalogue.participant.headquarter_address?.country"
                      class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                      <div class="text-xs font-semibold text-base-content/60">Country</div>
                      <div class="sm:col-span-2 text-sm font-mono break-all">{{
                        displayValue(catalogue.participant.headquarter_address?.country) }}</div>
                    </div>
                    <div v-if="catalogue.participant.headquarter_address?.locality"
                      class="grid grid-cols-1 sm:grid-cols-3 gap-1 sm:gap-4">
                      <div class="text-xs font-semibold text-base-content/60">Locality</div>
                      <div class="sm:col-span-2 text-sm font-mono break-all">{{
                        displayValue(catalogue.participant.headquarter_address?.locality) }}</div>
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
          <div class="card bg-base-100 shadow-sm border border-base-300">
            <div class="card-body">
              <p class="text-sm text-base-content/70">
                This template is free to use.
              </p>
              <div class="pt-2 flex flex-col gap-2">
                <RouterLink class="btn btn-sm btn-primary rounded-box"
                  :to="{ name: ROUTES.TEMPLATE_CATALOGUES.NEGOTIATION_CREATE, params: { did } }">
                  Create Negotiation
                </RouterLink>
                <button class="btn btn-sm btn-ghost rounded-box" @click="router.back()">
                  Back
                </button>
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
import { ROUTES } from '@/router/router'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const error = ref<string | null>(null)
const catalogue = ref<TemplateResource | null>(null)

const did = computed(() => `${route.params.did ?? ''}`)

async function load() {
  loading.value = true
  error.value = null
  try {
    catalogue.value = await templateCatalogueIntegrationService.retrieve_template_by_id({ did: did.value })
  } catch (e: any) {
    error.value = e?.message || 'Error loading template catalogue'
  } finally {
    loading.value = false
  }
}

load()

function displayValue(value: unknown): string {
  return value === null || value === undefined || value === '' ? '' : String(value)
}

function displayDate(value: unknown): string {
  if (value === null || value === undefined || value === '') return ''
  const d = new Date(String(value))
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleDateString()
}

</script>