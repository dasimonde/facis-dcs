import { defineStore } from 'pinia'
import { type Ref, ref } from 'vue'
import { type RouteLocationNormalized, useRouter } from 'vue-router'
import { ROUTES } from '@/router/router'

export const useNavStore = defineStore('nav', () => {
  const previousRoute: Ref<RouteLocationNormalized | null> = ref(null)
  const router = useRouter()

  async function goToPreviousRoute() {
    if (!previousRoute.value) {
      await router.push({ name: ROUTES.HOME })
    } else {
      await router.push(previousRoute.value)
    }
  }

  return { previousRoute, goToPreviousRoute }
})
