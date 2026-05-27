<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">

    <!-- Tabs -->
    <div class="sticky top-0 z-10 shrink-0 bg-base-100 border-b border-base-300">
      <div class="max-w-4xl mx-auto px-6 pt-3">
        <p class="text-xs font-black uppercase tracking-widest text-base-content/40 mb-2">
          View Template Catalogue
        </p>
        <div role="tablist" class="tabs tabs-border tabs-lg">
          <a
            v-for="tab in tabs"
            :key="tab.id"
            role="tab"
            class="tab"
            :class="{ 'tab-active text-primary': activeTab === tab.id }"
            @click="setActiveTab(tab.id)"
          >
            {{ tab.label }}
          </a>
        </div>
      </div>
    </div>

    <!-- Tab Content -->
    <div class="grow mt-5">
      <div class="max-w-4xl mx-auto p-6">
        <div v-if="loading" class="px-4">Loading Template Catalogue...</div>
        <div v-else-if="error" class="px-4">{{ error }}</div>
        <div v-else>
          <CatalogueTemplateDetailsInfo v-show="activeTab === 'details'" />
          <CatalogueTemplateMetaDataInfo v-show="activeTab === 'meta'" />
          <CatalogueTemplatePreviewInfo v-show="activeTab === 'preview'" />
        </div>
      </div>
    </div>

    <!-- Pinned Footer -->
    <div
      v-if="did"
      class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100"
    >
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-outline md:w-32" @click="router.back()">Back</button>
        <button
          class="btn btn-primary flex-1"
          :disabled="isRegisterDisabled"
          @click="registerTemplate"
        >
          <span v-if="registerLoading" class="loading loading-spinner loading-sm"></span>
          Register
        </button>
      </div>
    </div>

    <ConfirmationModal ref="confirmation-modal" />
  </div>
</template>

<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { contractTemplateService } from '@/services/contract-template-service'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import { useAuthStore } from '@/stores/auth-store'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import type { TemplateCatalogueRetrieveByIdResponse } from '@/models/responses/template-catalogue-integration-response'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import CatalogueTemplateDetailsInfo from '@/modules/template-catalogue/components/CatalogueTemplateDetailsInfo.vue'
import CatalogueTemplateMetaDataInfo from '@/modules/template-catalogue/components/CatalogueTemplateMetaDataInfo.vue'
import CatalogueTemplatePreviewInfo from '@/modules/template-catalogue/components/CatalogueTemplatePreviewInfo.vue'
import { isSameTemplateDataRef } from '@template-repository/utils/template-data-ref'
import { TemplateState } from '@/types/contract-template-state'
import { ROUTES } from '@/router/router'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const did = computed(() => (typeof route.params.did === 'string' ? route.params.did : ''))

const loading = ref(false)
const error = ref<string | null>(null)
const catalogue = ref<TemplateCatalogueRetrieveByIdResponse | null>(null)

const registerLoading = ref(false)

const draftStore = useTemplateDraftStore()

type CatalogueTabId = 'details' | 'meta' | 'preview'
const activeTab = ref<CatalogueTabId>('details')

const tabs: { id: CatalogueTabId; label: string }[] = [
  { id: 'details' as const, label: 'Details' },
  { id: 'meta' as const, label: 'Meta Data' },
  { id: 'preview' as const, label: 'Preview' },
]

const templatesStore = useContractTemplatesStore()
const { contractTemplates, loading: localTemplatesLoading } = storeToRefs(templatesStore)
const templateManager = computed(() => authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false)

// TODO: backend may need to provide a more direct way to check if the catalogue template is already registered locally
const hasLocalTemplate = computed(() => {
  if (!catalogue.value) return false

  const catalogueRef = {
    templateId: catalogue.value.did,
    version: catalogue.value.version,
    document_number: catalogue.value.document_number,
  }

  return contractTemplates.value.some((t) =>
    isSameTemplateDataRef(catalogueRef, {
      templateId: t.did,
      version: t.version,
      document_number: t.document_number,
    }),
  )
})

const isRegisterDisabled = computed(() => {
  if (!templateManager.value) return true
  if (localTemplatesLoading.value) return true
  if (!catalogue.value) return true
  if (!catalogue.value.updated_at) return true
  return hasLocalTemplate.value || registerLoading.value
})

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

watch(
  () => did.value,
  async () => {
    if (!did.value) return
    loading.value = true
    error.value = null
    activeTab.value = 'details'

    try {
      const data = await templateCatalogueIntegrationService.retrieve_template_by_id({ did: did.value })
      if (!data) {
        error.value = 'No catalogue template found'
        catalogue.value = null
        return
      }
      catalogue.value = data

      const templateData = data.template_data
      if (!templateData) {
        error.value = 'Template data is missing from catalogue response'
        return
      }

      draftStore.reset({
        workflow: 'template',
        did: data.did,
        name: data.name ?? '',
        description: data.description ?? '',
        templateDataVersion: templateData.templateDataVersion ?? 1,
        documentOutline: templateData.documentOutline ?? [],
        documentBlocks: templateData.documentBlocks ?? [],
        semanticConditions: templateData.semanticConditions ?? [],
        customMetaData: templateData.customMetaData ?? [],
        subTemplateSnapshots: templateData.subTemplateSnapshots ?? [],
        templateType: data.template_type as any,
        state: TemplateState.draft,
        document_number: data.document_number ?? null,
        version: data.version ?? null,
        updated_at: data.updated_at ?? null,
        created_by: '',
        responsible_persons: null,
      })
    } catch (e: any) {
      error.value = e?.message || 'Error loading template catalogue'
      catalogue.value = null
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

onMounted(() => {
  if (!contractTemplates.value.length && !localTemplatesLoading.value) {
    templatesStore.loadTemplates()
  }
})

function setActiveTab(tabId: CatalogueTabId) {
  activeTab.value = tabId
}

async function registerTemplate() {
  if (!catalogue.value?.updated_at) return

  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with registration?' })
    if (isCanceled) return

    registerLoading.value = true
    await contractTemplateService.register({
      did: catalogue.value.did,
      updated_at: catalogue.value.updated_at,
    })

    await templatesStore.loadTemplates()
    router.push({ name: ROUTES.TEMPLATES.VIEW, params: { did: catalogue.value.did } })
  } catch (e: any) {
    error.value = e?.message || 'Registration failed'
  } finally {
    registerLoading.value = false
  }
}
</script>

