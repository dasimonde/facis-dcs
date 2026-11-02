<script setup lang="ts">
import { computed, normalizeClass, ref, useAttrs, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { TemplateType } from '@template-repository/models/contract-template'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { type ContractTemplateState, TemplateState } from '@/types/contract-template-state'
import { reportActionError } from '@/utils/report-action-error'
import type { PartialContractTemplate } from '@/models/contract-template/contract-template'

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

// DCS-FR-TR-17, DCS-FR-TR-18: a registered or published template is
// deprecated; a draft, rejected or deprecated one is deleted. A template in
// review or approved has no archive action.
const deprecatedStates: ContractTemplateState[] = [TemplateState.registered, TemplateState.published]
const deletedStates: ContractTemplateState[] = [TemplateState.draft, TemplateState.rejected, TemplateState.deprecated]
const deprecates = computed(() => deprecatedStates.includes(props.template.state))
const deletes = computed(() => deletedStates.includes(props.template.state))
const canArchive = computed(() => isManager.value && (deprecates.value || deletes.value))

const archiveLabel = computed(() => (deprecates.value ? 'Deprecate' : 'Delete'))

const showPublishButton = computed(() => {
  return (
    isManager.value &&
    props.template.state === TemplateState.registered &&
    props.template.template_type === TemplateType.contractTemplate
  )
})

// DCS-IR-TR-06: only a template that passed the policy audit at approval is registered.
const showRegisterButton = computed(() => {
  return (
    isManager.value &&
    props.template.state === TemplateState.approved &&
    props.template.template_type === TemplateType.contractTemplate &&
    props.template.compliance_status === 'COMPLIANT'
  )
})

const archive = async () => {
  try {
    if (!confirmationModal.value) return
    // DCS-IR-TR-07: an archival requires a justification.
    const { isCanceled, data } = await confirmationModal.value.reveal({
      message: deprecates.value
        ? 'Deprecate this template? No new contract can be made from it.'
        : 'Delete this template? Contracts that are not terminated block the deletion.',
      confirmLabel: archiveLabel.value,
      editor: { requiredText: true, label: 'Justification', placeholder: 'Why is the template archived?' },
    })
    if (!isCanceled && data) {
      await contractTemplateService.archive({
        did: props.template.did,
        updated_at: props.template.updated_at,
        justification: data.trim(),
      })
      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    reportActionError(err, `${archiveLabel.value} template`)
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
    reportActionError(err, 'Publish template')
  } finally {
    isPublishing.value = false
  }
}

async function register() {
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with registration?' })
    if (!isCanceled) {
      await contractTemplateService.register({ did: props.template.did })

      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    reportActionError(err, 'Register template')
  }
}
</script>

<template>
  <button v-if="showRegisterButton" :class="$attrs.class" @click="register">Register</button>
  <button v-if="showPublishButton" :class="$attrs.class" :disabled="isPublishing" @click="publish">
    <span v-if="isPublishing" class="loading loading-sm loading-spinner"></span>
    Publish
  </button>
  <button v-if="canArchive" :class="[filteredClass, 'btn-error']" @click="archive">{{ archiveLabel }}</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
