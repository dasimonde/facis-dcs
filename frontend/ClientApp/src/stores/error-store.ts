import { defineStore } from 'pinia'
import { ref, type Ref } from 'vue'

type ErrorType = 'error' | 'info'

interface ErrorMessage {
  id: number
  type: ErrorType
  message: string
}

export const useErrorStore = defineStore('error', () => {
  const errors: Ref<ErrorMessage[]> = ref([])
  let nextId = 0

  function add(message: string, type: ErrorType = 'error', duration: number = 4000) {
    const id = nextId++
    errors.value.push({ id, type, message })

    setTimeout(() => remove(id), duration)
  }

  function remove(id: number) {
    errors.value = errors.value.filter((err) => err.id !== id)
  }

  return { errors, add, remove }
})
