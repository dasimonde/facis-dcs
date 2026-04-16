<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState, type ContractTemplateState } from '@/types/contract-template-state'
import { computed, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'

defineOptions({
  inheritAttrs: false,
})

const props = defineProps<{
  item: PartialContractTemplate
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const authStore = useAuthStore()

const isManager = computed(() => {
  return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
})

const canArchive = computed(() => {
  const archiveStates: ContractTemplateState[] = [TemplateState.deleted, TemplateState.deprecated]
  return isManager.value && !archiveStates.includes(props.item.state)
})

const canRegister = computed(() => {
  return isManager.value && props.item.state === TemplateState.approved
})

const archive = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with archiving?' })
    if (!isCanceled) {
      await contractTemplateService.archive({ did: props.item.did, updated_at: props.item.updated_at })
      router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Archiving failed:', err)
  }
}

const register = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with registration?' })
    if (!isCanceled) {
      await contractTemplateService.register({ did: props.item.did })
      router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Registration failed:', err)
  }
}

const audit = async () => {
  try {
    await contractTemplateService.audit({ did: props.item.did })
  } catch (err) {
    console.error('Audit failed:', err)
  }
}
</script>

<template>
  <button v-if="canRegister" :class="$attrs.class" @click="register">Register</button>
  <button v-if="canArchive" :class="[$attrs.class, 'hover:btn-error']" @click="archive">Archive</button>
  <button v-if="isManager" :class="$attrs.class" @click="audit">Audit</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
