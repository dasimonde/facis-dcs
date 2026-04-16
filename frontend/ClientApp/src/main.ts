import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import { router } from './router/router'
import { useErrorStore } from './stores/error-store'

const app = createApp(App).use(createPinia())

window.addEventListener('unhandledrejection', (event) => {
  const errorStore = useErrorStore()
  errorStore.add(event.reason)
})

app.config.errorHandler = (err, _instance, _info) => {
  const errorStore = useErrorStore()
  const message = err instanceof Error ? err.message : `Error: ${err ? String(err) : 'unknown'}`
  errorStore.add(message)
}

app.use(router)

app.mount('#app')
