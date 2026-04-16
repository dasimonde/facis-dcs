<script setup lang="ts">
import { BarsArrowDownIcon, BarsArrowUpIcon, ChevronUpDownIcon } from '@heroicons/vue/20/solid'
import { useTemplateRef } from 'vue'

const props = defineProps<{
  sorter: Map<string, string>
}>()

const sortPopover = useTemplateRef('sortPopover')

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
  <button id="list-btn-sort" class="btn btn-neutral m-2" :class="$attrs.class" popovertarget="list-popover-sort">
    <span>Sort by</span> <ChevronUpDownIcon class="w-6 h-6" />
  </button>
  <ul
    ref="sortPopover"
    class="dropdown dropdown-end menu w-52 rounded-box bg-base-300 shadow-sm"
    popover
    anchor="sort-btn"
    id="list-popover-sort"
  >
    <template v-for="[key, item] in sorter.entries()" :key="key">
      <li>
        <a @click="sortItemsBy(key)" class="flex justify-between w-full"
          ><span>{{ item }}</span
          ><ChevronUpDownIcon v-if="key !== sortBy" class="w-6 h-6" /><BarsArrowUpIcon
            v-else-if="sortOrder === 1"
            class="w-6 h-6" /><BarsArrowDownIcon v-else class="w-6 h-6"
        /></a>
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
