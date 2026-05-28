<script setup lang="ts">
import { BarsArrowDownIcon, BarsArrowUpIcon, ChevronUpDownIcon } from '@heroicons/vue/20/solid'
import { useTemplateRef } from 'vue'

const props = defineProps<{
  sorter: Map<string, string>
  disabled?: boolean
}>()

const sortPopover = useTemplateRef('sort-popover')

const sortBy = defineModel<string>('sortBy', { required: true })
const sortOrder = defineModel<number>('sortOrder', { required: true })

function sortItemsBy(key: string) {
  const newSorter = props.sorter.has(key) ? key : props.sorter.keys().next().value!
  sortOrder.value = sortBy.value === newSorter ? -sortOrder.value : 1
  sortBy.value = newSorter
  sortPopover.value?.hidePopover()
}
</script>

<template>
  <button
    id="list-btn-sort"
    class="btn m-2 btn-primary"
    :class="[$attrs.class, !!disabled ? 'btn-disabled' : '']"
    popovertarget="list-popover-sort"
    :disabled="!!disabled"
  >
    <span>Sort by</span>
    <ChevronUpDownIcon class="h-6 w-6" />
  </button>
  <ul
    id="list-popover-sort"
    ref="sort-popover"
    class="menu dropdown dropdown-end w-52 rounded-box bg-base-300 shadow-sm"
    popover
    anchor="sort-btn"
  >
    <template v-for="[key, item] in sorter.entries()" :key="key">
      <li>
        <a class="flex w-full justify-between" @click="sortItemsBy(key)">
          <span>{{ item }}</span>
          <ChevronUpDownIcon v-if="key !== sortBy" class="h-6 w-6" />
          <BarsArrowUpIcon v-else-if="sortOrder === 1" class="h-6 w-6" />
          <BarsArrowDownIcon v-else class="h-6 w-6" />
        </a>
      </li>
    </template>
  </ul>
</template>

<style scoped>
#list-btn-sort {
  anchor-name: --anchor-list-sort;
}

#list-popover-sort {
  position-anchor: --anchor-list-sort;
}
</style>
