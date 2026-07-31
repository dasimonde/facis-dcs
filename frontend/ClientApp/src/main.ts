import './style.css'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { bindHydraLoginChallengeFromURL } from './hydra-login-guard'
import { router } from './router/router'
import { useErrorStore } from './stores/error-store'

const app = createApp(App).use(createPinia())

window.addEventListener('unhandledrejection', (event) => {
  const errorStore = useErrorStore()
  errorStore.add(event.reason)
})

app.config.errorHandler = (err, _instance, _info) => {
  const errorStore = useErrorStore()
  const message = err instanceof Error ? err.message : `Error: ${err ? JSON.stringify(err) : 'unknown'}`
  errorStore.add(message)
}

app.use(router)

void bindHydraLoginChallengeFromURL().finally(() => {
  app.mount('#app')
})
