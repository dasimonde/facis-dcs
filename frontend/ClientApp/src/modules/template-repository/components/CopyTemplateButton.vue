<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useRouter } from 'vue-router'
import { useTemplateDraftStore } from '../store/templateDraftStore'
import { useTemplatePermissions } from '../composables/useTemplatePermissions'

const router = useRouter()
const draftStore = useTemplateDraftStore()

const { isCreator, isManager } = useTemplatePermissions()

const copyTemplate = async () => {
  if (!draftStore.did || (!isCreator && !isManager)) return

  const response = await contractTemplateService.copy({ did: draftStore.did })
  if (response.did) {
    await router.push({ name: ROUTES.TEMPLATES.EDIT, params: { did: response.did } })
  }
}
</script>

<template>
  <button @click="copyTemplate">Copy</button>
</template>
