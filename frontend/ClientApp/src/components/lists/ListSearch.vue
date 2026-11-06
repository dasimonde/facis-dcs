<script setup lang="ts" generic="T extends { did: string }">
import { Combobox, ComboboxInput, ComboboxOption, ComboboxOptions } from '@headlessui/vue'
import {
  computed,
  nextTick,
  onBeforeUnmount,
  type Ref,
  ref,
  type ShallowRef,
  shallowRef,
  useSlots,
  useTemplateRef,
  watch,
} from 'vue'

type FilterLabelConfig<T> = Partial<Record<keyof T, string>>
type SearchFunction<T> = (request: Record<string, unknown>) => Promise<T[]>

const props = defineProps<{
  items: T[]
  filterLabels: FilterLabelConfig<T>
  searchFn: SearchFunction<T>
  emptyItem: T
  placeholder?: string
  filters?: Record<string, unknown>
  extraRequest?: Record<string, unknown>
}>()

const emit = defineEmits<{
  searchResult: [value: T[] | null]
}>()

const slots = useSlots()

const searchQuery = ref('')
const isSearching = ref(false)
const searchError = ref<string | null>(null)

type FilterLabels = typeof props.filterLabels
type FilterLabelKey = keyof FilterLabels
type FilterLabelValue = FilterLabels[FilterLabelKey]

const selectedFilter = ref<FilterLabelValue>(Object.values(props.filterLabels)[0] ?? '')
const filterPopover = useTemplateRef('filter-popover')
const searchResults: ShallowRef<T[]> = shallowRef([])
// Bumped to drop stale in-flight searches when filters/extraRequest change.
let requestVersion = 0
onBeforeUnmount(() => requestVersion++)
const requestFilters = computed(() => ({
  ...Object.fromEntries(
    Object.entries({ ...props.extraRequest, ...props.filters }).filter(
      ([, value]) => value !== undefined && value !== null && value !== '',
    ),
  ),
  ...(searchQuery.value.trim() && searchKey.value ? { [searchKey.value]: searchQuery.value } : {}),
}))
const hasQuery = computed(() => Object.keys(requestFilters.value).length > 0)
watch(
  () => props.filters,
  () => {
    void publishSearch(searchRequest())
  },
  { deep: true, flush: 'sync' },
)

const selectedOption: Ref<T | null> = ref(null)

const searchKey = computed(() => {
  return (Object.keys(props.filterLabels) as FilterLabelKey[]).find(
    (key) => props.filterLabels[key] === selectedFilter.value,
  )
})

const hasTextQuery = computed(() => searchQuery.value.trim().length > 0)

watch(
  () => props.extraRequest,
  () => {
    requestVersion++
    searchResults.value = []
    searchError.value = null
    isSearching.value = false
  },
  { deep: true, flush: 'sync' },
)

const searchedItems = computed(() => {
  if (!hasQuery.value) return props.items

  if (searchResults.value.length === 0) return []

  return searchResults.value
})

const inputValue: Ref<T> = computed(() => {
  return !hasTextQuery.value || !searchKey.value
    ? props.emptyItem
    : { ...props.emptyItem, [searchKey.value]: searchQuery.value }
})

const isFilterSelectionDisabled = computed(() => Object.entries(props.filterLabels).length === 1)

function searchRequest(): Promise<boolean> {
  const version = ++requestVersion
  searchResults.value = []
  searchError.value = null
  isSearching.value = false
  if (!hasQuery.value) {
    return Promise.resolve(true)
  }
  isSearching.value = true
  const request = { ...requestFilters.value }
  return (async () => {
    try {
      const result = await props.searchFn(request)
      if (version !== requestVersion) return false
      searchResults.value = result
      return true
    } catch (error: unknown) {
      if (version !== requestVersion) return false
      searchError.value = error instanceof Error && error.message ? error.message : 'Search failed'
      return false
    } finally {
      if (version === requestVersion) isSearching.value = false
    }
  })()
}

async function publishSearch(run: Promise<boolean>) {
  const version = requestVersion
  if ((await run) && version === requestVersion) emit('searchResult', hasQuery.value ? searchResults.value : null)
}

const searchJoin = useTemplateRef('search-join')

function syncSearchQueryFromInput(event?: Event) {
  if (event?.target instanceof HTMLInputElement) {
    searchQuery.value = event.target.value
    return
  }
  const input = searchJoin.value?.querySelector<HTMLInputElement>('input')
  if (input && input.value !== searchQuery.value) searchQuery.value = input.value
}

async function searchList(event?: Event) {
  // Combobox @change can lag behind typing; sync before Search/Enter so an empty
  // Filters row still searches the text the user typed.
  syncSearchQueryFromInput(event)
  await publishSearch(searchRequest())
}

function inputDisplayValue(_item: T | null): string {
  return searchQuery.value
}

function autocompleteOptionClasses(active: boolean, selected: boolean) {
  return [
    'cursor-pointer px-4 py-2',
    active ? 'bg-secondary text-secondary-content' : 'bg-base-100',
    selected ? 'font-bold' : '',
  ]
}

async function onComboboxFocus() {
  await searchRequest()
}

function onSearchInput(event: Event) {
  searchQuery.value = (event.target as HTMLInputElement).value
}

async function onSearchChange(event: Event) {
  searchQuery.value = (event.target as HTMLInputElement).value
  const run = searchRequest()
  if (!searchQuery.value.trim()) await publishSearch(run)
  else await run
}

function onComboboxUpdate(item: T) {
  selectedOption.value = item
  if (selectedOption.value) {
    searchQuery.value = searchKey.value ? String(selectedOption.value[searchKey.value]) : ''
    void searchRequest()
  }
}

function onFilterSelect(label: FilterLabelValue) {
  selectedFilter.value = label
  void publishSearch(searchRequest())
  filterPopover.value?.hidePopover()
}

const showInitialFocus = ref(true)

function focusFirstOption() {
  void nextTick(() => {
    filterPopover.value?.querySelector<HTMLElement>('a[tabindex="0"]')?.focus()
  })
}

function handlePopoverToggle(event: ToggleEvent) {
  if (event.newState === 'closed') {
    showInitialFocus.value = true
  } else if (showInitialFocus.value) {
    focusFirstOption()
  }
}
</script>

<template>
  <div class="w-full min-w-0">
    <div ref="search-join" class="join m-2 flex w-full min-w-0 flex-col items-stretch sm:flex-row">
      <div class="join-item flex shrink-0 self-stretch">
        <button
          id="list-btn-search"
          type="button"
          class="select-button btn h-full min-h-0 w-full cursor-default rounded-t-md rounded-b-none border-secondary px-4 py-0 btn-outline-default sm:rounded-l-md sm:rounded-tr-none"
          popovertarget="list-popover-search"
          aria-haspopup="listbox"
          :aria-expanded="filterPopover?.matches(':popover-open')"
          :class="{ 'btn-disabled': isFilterSelectionDisabled }"
          :disabled="isFilterSelectionDisabled"
        >
          {{ selectedFilter }}
        </button>
        <ul
          id="list-popover-search"
          ref="filter-popover"
          class="menu dropdown dropdown-start mt-2 w-52 rounded-box bg-base-300 shadow-sm"
          popover
          role="listbox"
          :aria-label="'Select search filter'"
          @toggle="handlePopoverToggle"
        >
          <li role="option" aria-selected="false" class="menu-title">
            <span class="menu-disabled pointer-events-none text-base-content/70 select-none">Select search filter</span>
          </li>
          <template v-for="([key, label], index) in Object.entries(filterLabels)" :key="key">
            <li role="option" :aria-selected="label === selectedFilter">
              <a
                tabindex="0"
                :class="{
                  'bg-primary text-primary-content': label === selectedFilter,
                  'menu-focus': index === 0 && showInitialFocus,
                }"
                @blur="index === 0 ? (showInitialFocus = false) : null"
                @click="onFilterSelect(label)"
                @keydown.enter="onFilterSelect(label)"
                @keydown.space.prevent="onFilterSelect(label)"
              >
                {{ label }}
              </a>
            </li>
          </template>
        </ul>
      </div>
      <div class="relative min-w-0 grow self-stretch">
        <Combobox v-model="selectedOption" nullable @update:model-value="onComboboxUpdate">
          <label
            class="input join-item ms-0 -mt-px h-full min-h-0 w-full min-w-0 rounded-none input-secondary sm:-ms-px sm:mt-0"
          >
            <ComboboxInput
              :display-value="(item) => inputDisplayValue(item as T | null)"
              :placeholder="placeholder || 'Search'"
              class="w-full min-w-0 grow bg-transparent"
              :aria-label="placeholder || 'Search'"
              @input="onSearchInput"
              @change="onSearchChange"
              @focus="onComboboxFocus"
              @keydown.enter="searchList"
            />
          </label>

          <ComboboxOptions
            v-if="searchQuery.length > 0"
            class="absolute top-full right-0 left-0 z-10 rounded-lg border border-base-300 bg-base-100 shadow-lg"
          >
            <ComboboxOption :value="inputValue" class="hidden"></ComboboxOption>

            <div v-if="isSearching" class="px-4 py-2 text-base-content/50">Searching...</div>
            <div v-else-if="searchError" role="alert" class="px-4 py-2 text-error">{{ searchError }}</div>
            <template v-else-if="searchedItems.length > 0">
              <ComboboxOption
                v-for="item in searchedItems"
                :key="item.did"
                v-slot="{ active, selected }"
                :value="item"
                as="template"
              >
                <li v-if="searchKey" :class="autocompleteOptionClasses(active, selected)">
                  <span class="block truncate">{{ item[searchKey] }}</span>
                </li>
              </ComboboxOption>
            </template>

            <div v-else class="px-4 py-2 text-base-content/50">No templates found</div>
          </ComboboxOptions>
        </Combobox>
      </div>
      <slot name="before-search-button" :submit="searchList" />
      <button
        type="button"
        class="btn join-item ms-0 -mt-px h-full min-h-0 shrink-0 rounded-t-none rounded-b-md px-4 py-0 btn-secondary sm:-ms-px sm:mt-0 sm:rounded-r-md sm:rounded-bl-none"
        @mousedown.prevent
        @click="searchList"
      >
        Search
      </button>
    </div>
    <div
      v-if="searchError && !hasTextQuery"
      role="alert"
      class="px-4 text-sm text-error"
      data-testid="list-search-error"
    >
      {{ searchError }}
    </div>
    <div
      v-else-if="isSearching && !hasTextQuery && slots['before-search-button']"
      class="px-4 text-sm text-base-content/50"
      aria-live="polite"
    >
      Searching...
    </div>
  </div>
</template>

<style scoped>
.join {
  --list-search-control-height: calc(var(--size-field, 0.25rem) * 10);
}

.join :deep(.input.join-item),
.join :deep(.btn.join-item),
#list-btn-search {
  box-sizing: border-box;
  height: var(--list-search-control-height);
  min-height: var(--list-search-control-height);
  padding-block: 0;
}

#list-btn-search {
  anchor-name: --anchor-list-search;
}

#list-popover-search {
  position-anchor: --anchor-list-search;
}
</style>
