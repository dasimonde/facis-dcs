import { isAxiosError } from 'axios'
import { useErrorStore } from '@/stores/error-store'

const reportedHttpErrors = new WeakMap<object, number>()

export function bindReportedHttpError(error: object, messageId: number): void {
  reportedHttpErrors.set(error, messageId)
}

/**
 * Gives an action failure user-facing context while preserving the HTTP
 * interceptor as the single owner of Axios error toasts.
 */
export function reportActionError(error: unknown, context: string): void {
  const errorStore = useErrorStore()
  if (isAxiosError(error)) {
    const messageId = reportedHttpErrors.get(error)
    if (messageId !== undefined && errorStore.addContext(messageId, context)) return
  }

  const detail = error instanceof Error && error.message ? error.message : 'The action could not be completed.'
  errorStore.add(`${context}: ${detail}`)
}
