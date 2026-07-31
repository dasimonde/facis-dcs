import { defineStore } from 'pinia'
import { type Ref, ref } from 'vue'

type ErrorType = 'error' | 'info'

interface ErrorMessage {
  id: number
  type: ErrorType
  message: string
}

export const useErrorStore = defineStore('error', () => {
  const errors: Ref<ErrorMessage[]> = ref([])
  let nextId = 0

  function add(message: string, type: ErrorType = 'error', duration = 4000) {
    const id = nextId++
    errors.value.push({ id, type, message })

    setTimeout(() => remove(id), duration)
    return id
  }

  function remove(id: number) {
    errors.value = errors.value.filter((err) => err.id !== id)
  }

  function addContext(id: number, context: string) {
    const error = errors.value.find((candidate) => candidate.id === id)
    if (!error) return false
    if (!error.message.startsWith(`${context}:`)) {
      error.message = `${context}: ${error.message}`
    }
    return true
  }

  return { errors, add, remove, addContext }
})
