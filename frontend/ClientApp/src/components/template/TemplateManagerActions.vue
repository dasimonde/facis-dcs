<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { useContractPlainTextConverter } from '@/modules/contract-workflow-engine/composables/useContractPlainTextConverter'
import { toPdfData } from '@/modules/contract-workflow-engine/utils/contractPdfConverter'
import { downloadContractPdf } from '@/modules/contract-workflow-engine/utils/contractPdfExporter'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { templateCatalogueIntegrationService } from '@/services/template-catalogue-integration-service'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState, type ContractTemplateState } from '@/types/contract-template-state'
import { computed, normalizeClass, onMounted, ref, useAttrs, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'

defineOptions({
  inheritAttrs: false,
})

const attrs = useAttrs()
const { convertContractToPlainTextBlocks } = useContractPlainTextConverter()

const filteredClass = computed(() => {
  return normalizeClass(attrs.class)
    .split(' ')
    .filter(
      (cls) =>
        !['btn-primary', 'btn-secondary', 'btn-accent', 'btn-success', 'btn-warning', 'btn-error', 'btn-info'].includes(
          cls,
        ),
    )
    .join(' ')
})

const props = defineProps<{
  template: PartialContractTemplate
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const authStore = useAuthStore()

const isPublished = ref(false)
const isPublishedLoading = ref(true)
const isPublishing = ref(false)

const isManager = computed(() => {
  return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
})

const canArchive = computed(() => {
  const archiveStates: ContractTemplateState[] = [TemplateState.deleted, TemplateState.deprecated]
  return isManager.value && !archiveStates.includes(props.template.state)
})

const showPublishButton = computed(() => {
  return isManager.value && props.template.state === TemplateState.approved
})

const checkTemplatePublishedInFederatedCatalogue = async (): Promise<boolean> => {
  try {
    const template = await templateCatalogueIntegrationService.retrieve_template_by_id({
      did: props.template.did,
      version: props.template.version,
    })
    return template !== null
  } catch (err) {
    console.error('Failed to check template published state in Federated Catalogue:', err)
    return false
  }
}

const refreshPublishedState = async () => {
  if (props.template.state !== TemplateState.approved) {
    isPublished.value = false
    isPublishedLoading.value = false
    return
  }
  isPublishedLoading.value = true
  try {
    isPublished.value = await checkTemplatePublishedInFederatedCatalogue()
  } finally {
    isPublishedLoading.value = false
  }
}

onMounted(refreshPublishedState)
watch(() => [props.template.did, props.template.version, props.template.state] as const, refreshPublishedState)

const archive = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with archiving?' })
    if (!isCanceled) {
      await contractTemplateService.archive({ did: props.template.did, updated_at: props.template.updated_at })
      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Archiving failed:', err)
  }
}

const publish = async () => {
  if (isPublishedLoading.value || isPublished.value || isPublishing.value) return
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with publishing?' })
    if (!isCanceled) {
      isPublishing.value = true
      await contractTemplateService.publish({ did: props.template.did, updated_at: props.template.updated_at })
      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Publishing failed:', err)
  } finally {
    isPublishing.value = false
  }
}

const exportPdf = async () => {
  const template = await contractTemplateService.retrieveById({ did: props.template.did })
  if (!template) return
  const blocks = convertContractToPlainTextBlocks(template.template_data)
  const pdfData = toPdfData(blocks)
  const title = `${template.name ?? 'contract-template'}`
  const filename = `${title}.pdf`
  downloadContractPdf(pdfData, filename, title)
}
</script>

<template>
  <button :class="$attrs.class" @click="exportPdf">Export PDF</button>
  <button
    v-if="showPublishButton"
    :class="$attrs.class"
    :disabled="isPublishedLoading || isPublished || isPublishing"
    @click="publish"
  >
    <span v-if="isPublishing" class="loading loading-sm loading-spinner"></span>
    {{ isPublishedLoading ? 'Publish' : isPublished ? 'Published' : 'Publish' }}
  </button>
  <button v-if="canArchive" :class="[filteredClass, 'btn-error']" @click="archive">Archive</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
