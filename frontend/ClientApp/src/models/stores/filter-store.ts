import type { ComputedRef, Ref } from "vue"

export interface FilterStore<T> {
  stateFilters: Ref<Set<T>>
  hasFilters: ComputedRef<boolean>
  hasFilter(filter: T): boolean
  setFilter(filter: T): void
  removeFilter(filter: T): void
  reset(): void
}
