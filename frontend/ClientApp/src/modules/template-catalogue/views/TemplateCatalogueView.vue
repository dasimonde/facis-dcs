<script setup lang="ts">
import { TemplateType, type TemplateTypeValue } from '@template-repository/models/contract-template'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'
import CatalogueTemplateDetailsInfo from '@/modules/template-catalogue/components/catalogue-template/CatalogueTemplateDetailsInfo.vue'
import CatalogueTemplateMetaDataInfo from '@/modules/template-catalogue/components/catalogue-template/CatalogueTemplateMetaDataInfo.vue'
import CatalogueTemplatePreviewInfo from '@/modules/template-catalogue/components/catalogue-template/CatalogueTemplatePreviewInfo.vue'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { TemplateState } from '@/types/contract-template-state'
import type { TemplateCatalogueRetrieveByIdResponse } from '@/models/responses/template-catalogue-integration-response'

const router = useRouter()
const route = useRoute()

const did = computed(() => (typeof route.params.did === 'string' ? route.params.did : ''))
const version = computed(() => {
  const raw = route.query.version
  const value = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isFinite(value) ? value : null
})

const loading = ref(false)
const error = ref<string | null>(null)
const remoteTemplateUnreachable = ref(false)
const catalogue = ref<TemplateCatalogueRetrieveByIdResponse | null>(null)

const registerLoading = ref(false)

const draftStore = useDcsDraftStore()

type CatalogueTabId = 'details' | 'meta' | 'preview'
const activeTab = ref<CatalogueTabId>('details')

const tabs: { id: CatalogueTabId; label: string }[] = [
  { id: 'details' as const, label: 'Details' },
  { id: 'meta' as const, label: 'Meta Data' },
  { id: 'preview' as const, label: 'Preview' },
]

const templatesStore = useContractTemplatesStore()
const { contractTemplates, loading: localTemplatesLoading } = storeToRefs(templatesStore)

const { isManager } = useContractPermissions()

const isRegisterDisabled = computed(() => {
  if (!isManager.value) return true
  if (localTemplatesLoading.value) return true
  if (!catalogue.value) return true
  if (catalogue.value.version == null) return true

  const result = contractTemplates.value.filter((contract) => contract.did === did.value)
  if (result.length > 0) {
    return true
  }

  return registerLoading.value
})

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

function toTemplateType(value: string | undefined): TemplateTypeValue {
  if (value === TemplateType.contractTemplate || value === TemplateType.component) {
    return value
  }
  return TemplateType.component
}

watch(
  () => [did.value, version.value] as const,
  async () => {
    if (!did.value || version.value == null) {
      error.value = 'Missing required URL query: version'
      catalogue.value = null
      return
    }
    loading.value = true
    error.value = null
    remoteTemplateUnreachable.value = false
    activeTab.value = 'details'

    try {
      const data = await templateCatalogueIntegrationService.retrieve_template_by_id({
        did: did.value,
        version: version.value,
      })
      if (!data) {
        error.value = 'No catalogue template found'
        catalogue.value = null
        return
      }
      catalogue.value = data

      const templateData = data.template_data
      if (!templateData) {
        remoteTemplateUnreachable.value = true
      }

      draftStore.loadDocument(templateData ?? {}, {
        did: data.did,
        name: data.name ?? '',
        description: data.description ?? '',
        templateType: toTemplateType(data.template_type),
        state: TemplateState.draft,
        document_number: data.document_number ?? null,
        version: data.version ?? null,
        updated_at: data.updated_at ?? null,
        created_by: '',
      })
    } catch (e: unknown) {
      error.value = e instanceof Error && e.message ? e.message : 'Error loading template catalogue'
      catalogue.value = null
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

onMounted(() => {
  void templatesStore.loadTemplates()
})

function setActiveTab(tabId: CatalogueTabId) {
  activeTab.value = tabId
}

async function registerTemplate() {
  if (catalogue.value?.version == null) return

  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with registration?' })
    if (isCanceled) return

    registerLoading.value = true
    const registered = await contractTemplateService.register({
      did: catalogue.value.did,
      version: catalogue.value.version,
    })

    await templatesStore.loadTemplates()
    await router.push({ name: ROUTES.TEMPLATES.EDIT, params: { did: registered.did } })
  } catch (e: unknown) {
    error.value = e instanceof Error && e.message ? e.message : 'Registration failed'
  } finally {
    registerLoading.value = false
  }
}
</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <!-- Tabs -->
    <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
      <div class="mx-auto max-w-4xl px-6 pt-3">
        <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">View Template Catalogue</p>
        <div role="tablist" class="tabs-border tabs tabs-lg">
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
    <div class="mt-5 grow">
      <div class="mx-auto max-w-4xl p-6">
        <div v-if="loading" class="px-4">Loading Template Catalogue...</div>
        <div v-else-if="error" class="px-4">{{ error }}</div>
        <div v-else>
          <p
            v-if="remoteTemplateUnreachable"
            class="mb-4 rounded-lg border border-base-300 bg-base-100 p-4 text-base-content"
          >
            Remote template source is currently unreachable.
          </p>
          <CatalogueTemplateDetailsInfo v-show="activeTab === 'details'" />
          <CatalogueTemplateMetaDataInfo v-show="activeTab === 'meta'" />
          <CatalogueTemplatePreviewInfo v-show="activeTab === 'preview'" />
        </div>
      </div>
    </div>

    <!-- Pinned Footer -->
    <div v-if="did" class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="router.back()">Back</button>
        <button class="btn flex-1 btn-primary" :disabled="isRegisterDisabled" @click="registerTemplate">
          <span v-if="registerLoading" class="loading loading-sm loading-spinner"></span>
          Register
        </button>
      </div>
    </div>

    <ConfirmationModal ref="confirmation-modal" />
  </div>
</template>
