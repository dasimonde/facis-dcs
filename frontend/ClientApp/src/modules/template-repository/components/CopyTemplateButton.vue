<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'

const router = useRouter()
const draftStore = useDcsDraftStore()

const { isCreator, isManager } = useTemplatePermissions()

const copyTemplate = async () => {
  if (!draftStore.did || (!isCreator && !isManager)) return
  try {
    const response = await contractTemplateService.copy({ did: draftStore.did })
    if (response.did) {
      await router.push({ name: ROUTES.TEMPLATES.EDIT, params: { did: response.did } })
    }
  } catch (err) {
    console.error('Copying failed:', err)
  }
}
</script>

<template>
  <button @click="copyTemplate">Copy</button>
</template>
