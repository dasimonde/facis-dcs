<script setup lang="ts" generic="T extends { did: string }">
import { Combobox, ComboboxInput, ComboboxOption, ComboboxOptions } from '@headlessui/vue'
import { computed, nextTick, type Ref, ref, type ShallowRef, shallowRef, useTemplateRef } from 'vue'

type FilterLabelConfig<T> = Partial<Record<keyof T, string>>
type SearchFunction<T> = (request: Record<string, unknown>) => Promise<T[]>

const props = defineProps<{
  items: T[]
  filterLabels: FilterLabelConfig<T>
  searchFn: SearchFunction<T>
  emptyItem: T
  placeholder?: string
}>()

const emit = defineEmits<{
  searchResult: [value: T[] | null]
}>()

const searchQuery = ref('')
const isSearching = ref(false)
const searchError = ref<string | null>(null)

type FilterLabels = typeof props.filterLabels
type FilterLabelKey = keyof FilterLabels
type FilterLabelValue = FilterLabels[FilterLabelKey]

const selectedFilter = ref<FilterLabelValue>(Object.values(props.filterLabels)[0] ?? '')
const filterPopover = useTemplateRef('filter-popover')
const searchResults: ShallowRef<T[]> = shallowRef([])
// The search currently in flight, so a caller that has to publish a result can
// wait for it rather than read searchResults mid-request.
let pendingSearch: Promise<void> | null = null

const selectedOption: Ref<T | null> = ref(null)

const searchKey = computed(() => {
  return (Object.keys(props.filterLabels) as FilterLabelKey[]).find(
    (key) => props.filterLabels[key] === selectedFilter.value,
  )
})

const searchedItems = computed(() => {
  if (searchQuery.value.length < 1) return props.items

  if (searchResults.value.length === 0) return []

  return searchResults.value
})

const inputValue: Ref<T> = computed(() => {
  return searchQuery.value.length < 1 || !searchKey.value
    ? props.emptyItem
    : { ...props.emptyItem, [searchKey.value]: searchQuery.value }
})

const isFilterSelectionDisabled = computed(() => Object.entries(props.filterLabels).length === 1)

async function searchRequest() {
  if (searchQuery.value.length < 1 || !searchKey.value) {
    searchResults.value = []
    pendingSearch = null
    return
  }

  isSearching.value = true
  searchError.value = null
  const run = (async () => {
    try {
      await retrieveSearch()
    } catch (e: unknown) {
      // A search that could not run has NOT established that nothing matches;
      // reporting it as an empty result is how a 403 reads as "no results".
      searchResults.value = []
      searchError.value = e instanceof Error && e.message ? e.message : 'Search failed'
    } finally {
      isSearching.value = false
    }
  })()
  pendingSearch = run
  await run
}

async function retrieveSearch() {
  if (!searchKey.value) return
  const request = { [searchKey.value]: searchQuery.value }
  searchResults.value = await props.searchFn(request)
}

async function searchList(event?: Event) {
  if (event && event.target instanceof HTMLInputElement && event.target.value !== searchQuery.value) {
    await searchRequest()
  } else if (searchQuery.value.trim().length > 0) {
    // Typing already started a search, and clicking Search is not an input
    // event, so nothing above awaits it. Emitting now publishes the empty
    // searchResults the arriving response never gets to correct.
    await (pendingSearch ?? searchRequest())
  }
  if (searchError.value) return
  if (searchQuery.value.trim().length > 0) {
    emit('searchResult', searchedItems.value)
  } else {
    emit('searchResult', null)
  }
}

function getDisplayValue(template: T | null): string {
  return searchKey.value && template ? String(template[searchKey.value]) : ''
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

async function onSearchChange(event: Event) {
  searchQuery.value = (event.target as HTMLInputElement).value
  if (searchQuery.value.trim().length === 0) {
    emit('searchResult', null)
  } else {
    await searchRequest()
  }
}

function onComboboxUpdate(item: T) {
  selectedOption.value = item
  if (selectedOption.value) {
    searchQuery.value = searchKey.value ? String(selectedOption.value[searchKey.value]) : ''
  }
}

function onFilterSelect(label: FilterLabelValue) {
  selectedFilter.value = label
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
  <div class="join m-2 flex-col sm:flex-row">
    <div class="join-item">
      <button
        id="list-btn-search"
        type="button"
        class="select-button btn w-full cursor-default rounded-t-md rounded-b-none border-secondary btn-outline-default sm:rounded-l-md sm:rounded-tr-none"
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
    <div class="relative grow">
      <Combobox v-model="selectedOption" nullable @update:model-value="onComboboxUpdate">
        <label class="input join-item ms-0 -mt-px w-full rounded-none input-secondary sm:-ms-px sm:mt-0">
          <ComboboxInput
            :display-value="(item) => getDisplayValue(item as T | null)"
            :placeholder="placeholder || 'Search'"
            class="w-full bg-transparent"
            :aria-label="placeholder || 'Search'"
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
    <button
      type="button"
      class="btn join-item ms-0 -mt-px rounded-t-none rounded-b-md btn-secondary sm:-ms-px sm:mt-0 sm:rounded-r-md sm:rounded-bl-none"
      @click="searchList"
    >
      Search
    </button>
  </div>
</template>

<style scoped>
#list-btn-search {
  anchor-name: --anchor-list-search;
}

#list-popover-search {
  position-anchor: --anchor-list-search;
}
</style>
