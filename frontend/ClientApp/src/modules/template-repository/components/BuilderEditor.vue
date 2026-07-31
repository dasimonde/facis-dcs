<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import EditorBlocks from '@template-repository/components/builder-editor/EditorBlocks.vue'
import EmptyBlockCreator from '@template-repository/components/builder-editor/EmptyBlockCreator.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'

const { layout } = storeToRefs(useDcsDraftStore())

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))
const hasBlocks = computed(() => (rootBlock.value?.['dcs:children']['@list'].length ?? 0) > 0)
</script>

<template>
  <div class="flex flex-col gap-4">
    <EmptyBlockCreator v-if="!hasBlocks" />
    <EditorBlocks v-else />
  </div>
</template>
