<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { useTemplatePermissions } from '@/modules/template-repository/composables/useTemplatePermissions'
import { TemplateType } from '@/modules/template-repository/models/contract-template'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useContractTemplatesStore } from '@/stores/contract-templates-store'
import { TemplateState, type ContractTemplateState } from '@/types/contract-template-state'
import { computed, normalizeClass, ref, useAttrs, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'

defineOptions({
  inheritAttrs: false,
})

const attrs = useAttrs()

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

const { isManager } = useTemplatePermissions()

const isPublishing = ref(false)

const templatesStore = useContractTemplatesStore()

const canArchive = computed(() => {
  const archiveStates: ContractTemplateState[] = [TemplateState.deleted, TemplateState.deprecated]
  return isManager.value && !archiveStates.includes(props.template.state)
})

const showPublishButton = computed(() => {
  return (
    isManager.value &&
    props.template.state === TemplateState.registered &&
    props.template.template_type === TemplateType.contractTemplate
  )
})

const showRegisterButton = computed(() => {
  return (
    isManager.value &&
    props.template.state === TemplateState.approved &&
    props.template.template_type === TemplateType.contractTemplate
  )
})

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
  if (isPublishing.value) return
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

async function register() {
  try {
    const registered = await contractTemplateService.register({
      did: props.template.did,
    })

    await templatesStore.loadTemplates()
    await router.push({ name: ROUTES.TEMPLATES.LIST, params: { did: registered.did } })
  } catch {}
}
</script>

<template>
  <button v-if="showRegisterButton" :class="$attrs.class" @click="register">Register</button>
  <button v-if="showPublishButton" :class="$attrs.class" :disabled="isPublishing" @click="publish">
    <span v-if="isPublishing" class="loading loading-sm loading-spinner"></span>
    Publish
  </button>
  <button v-if="canArchive" :class="[filteredClass, 'btn-error']" @click="archive">Archive</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
