import { useRouter } from 'vue-router'

export function useSyncPageTitle() {
  const router = useRouter()

  function setPageTitle() {
    const route = router.currentRoute.value
    document.title = route.meta?.title ?? 'DCS'
  }

  void router.isReady().then(setPageTitle)
  router.afterEach(setPageTitle)
}
