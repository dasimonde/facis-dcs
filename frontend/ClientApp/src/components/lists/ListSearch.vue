<script setup lang="ts" generic="T extends { did: string }">
import { Combobox, ComboboxInput, ComboboxOption, ComboboxOptions } from '@headlessui/vue'
import { computed, ref, useTemplateRef, type Ref } from 'vue'

type FilterLabelConfig<T> = Partial<Record<keyof T, string>>
type SearchFunction<T> = (request: Record<string, any>) => Promise<T[]>

const props = defineProps<{
  items: T[]
  filterLabels: FilterLabelConfig<T>
  searchFn: SearchFunction<T>
  emptyItem: T
  placeholder?: string
}>()

const emit = defineEmits<{
  searchResult: [value: T[]]
}>()

const searchQuery = ref('')
const isSearching = ref(false)

type FilterLabels = typeof props.filterLabels
type FilterLabelKey = keyof FilterLabels
type FilterLabelValue = FilterLabels[FilterLabelKey]

const selectedFilter = ref<FilterLabelValue>(
  (Object.values(props.filterLabels)[0] as FilterLabelValue) ?? ('' as FilterLabelValue),
)
const filterPopover = useTemplateRef('filterPopover')
const searchResults: Ref<T[]> = ref([])

const selectedOption: Ref<T | null> = ref(null)

const searchKey = computed(() => {
  return (Object.keys(props.filterLabels) as FilterLabelKey[]).find(
    (key) => props.filterLabels[key] === selectedFilter.value,
  )
})

const searchedItems = computed(() => {
  if (searchQuery.value.length < 1) return props.items

  if (searchResults.value.length === 0) return []

  const backendIds = new Set(searchResults.value.map((item) => item.did))

  return props.items.filter((item) => backendIds.has(item.did))
})

const inputValue: Ref<T> = computed(() => {
  return searchQuery.value.length < 1 || !searchKey.value
    ? props.emptyItem
    : { ...props.emptyItem, [searchKey.value]: searchQuery.value }
})

async function searchRequest() {
  if (searchQuery.value.length < 1 || !searchKey.value) {
    searchResults.value = []
    return
  }

  isSearching.value = true
  try {
    await retrieveSearch()
  } finally {
    isSearching.value = false
  }
}

async function retrieveSearch() {
  if (!searchKey.value) return
  const request = { [searchKey.value]: searchQuery.value }
  searchResults.value = await props.searchFn(request)
}

async function searchList(event?: Event) {
  if (event && event.target instanceof HTMLInputElement) {
    if (event.target.value !== searchQuery.value) {
      await searchRequest()
    }
  }
  emit('searchResult', searchedItems.value)
}

const getDisplayValue = (template: T | null): string => {
  return searchKey.value && template ? String(template[searchKey.value]) : ''
}

const autocompleteOptionClasses = (active: boolean, selected: boolean) => [
  'cursor-pointer px-4 py-2',
  active ? 'bg-secondary text-secondary-content' : 'bg-base-100',
  selected ? 'font-bold' : '',
]

async function onComboboxFocus() {
  await searchRequest()
}

function onSearchChange(event: Event) {
  searchQuery.value = (event.target as HTMLInputElement).value
  searchRequest()
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
</script>

<template>
  <div class="join m-2 flex-col sm:flex-row">
    <div class="join-item">
      <button
        id="list-btn-search"
        type="button"
        class="select select-secondary w-full rounded-t-md rounded-b-none sm:rounded-l-md sm:rounded-tr-none"
        popovertarget="list-popover-search"
        :class="{ 'btn-disabled': Object.entries(filterLabels).length === 1 }"
      >
        {{ selectedFilter }}
      </button>
      <ul
        ref="filterPopover"
        class="dropdown dropdown-start menu w-52 rounded-box bg-base-300 shadow-sm"
        popover
        id="list-popover-search"
      >
        <li class="menu-title">
          <span class="menu-disabled pointer-events-none select-none">Select search filter</span>
        </li>
        <template v-for="[key, label] in Object.entries(filterLabels)" :key="key">
          <li>
            <a :class="{ 'bg-primary text-primary-content': label === selectedFilter }" @click="onFilterSelect(label)">
              {{ label }}
            </a>
          </li>
        </template>
      </ul>
    </div>
    <div class="relative grow">
      <Combobox v-model="selectedOption" @update:model-value="onComboboxUpdate" nullable>
        <label class="input input-secondary join-item w-full rounded-none -mt-px ms-0 sm:mt-0 sm:-ms-px">
          <ComboboxInput
            @change="onSearchChange"
            @focus="onComboboxFocus"
            @keydown.enter="searchList"
            :display-value="(item) => getDisplayValue(item as T | null)"
            :placeholder="placeholder || 'Search'"
            class="w-full bg-transparent"
          />
        </label>

        <ComboboxOptions
          v-if="searchQuery.length > 0"
          class="absolute left-0 right-0 top-full z-10 rounded-lg border border-base-300 bg-base-100 shadow-lg"
        >
          <ComboboxOption :value="inputValue" class="hidden"></ComboboxOption>

          <div v-if="isSearching" class="px-4 py-2 text-base-content/50">Searching...</div>
          <template v-else-if="searchedItems.length > 0">
            <ComboboxOption
              v-for="item in searchedItems"
              :key="item.did"
              :value="item"
              as="template"
              v-slot="{ active, selected }"
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
      @click="searchList"
      class="btn btn-secondary join-item rounded-b-md rounded-t-none sm:rounded-r-md sm:rounded-bl-none -mt-px ms-0 sm:mt-0 sm:-ms-px"
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
