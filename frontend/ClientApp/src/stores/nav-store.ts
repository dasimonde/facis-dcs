import { ROUTES } from '@/router/router'
import { defineStore } from 'pinia'
import { ref, type Ref } from 'vue'
import { useRouter, type RouteLocationNormalized } from 'vue-router'

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
