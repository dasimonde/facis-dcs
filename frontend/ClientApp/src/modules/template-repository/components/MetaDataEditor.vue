<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, reactive, ref } from 'vue'
import MetaDataRow from '@template-repository/components/meta-data/MetaDataRow.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { customMetaData } = storeToRefs(store)

const allNames = computed(() => customMetaData.value.map((m) => m.name ?? ''))

const draft = reactive({
  name: '',
  value: '',
})

const addRowKey = ref(0)
const activeIndex = ref<number | null>(null)

function resetDraft() {
  draft.name = ''
  draft.value = ''
  addRowKey.value += 1
}

function setActiveIndex(index: number) {
  activeIndex.value = index
}

function createMeta(payload: { name: string; value: string }) {
  const ok = store.addMetaData(payload)
  if (ok) {
    resetDraft()
  }
}

function updateMeta(index: number, payload: { name: string; value: string }) {
  store.updateMetaData(index, payload)
}

function deleteMeta(index: number) {
  store.deleteMetaData(index)
}
</script>

<template>
  <div class="space-y-3">
    <div class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr class="text-base-content/70">
            <th class="w-1/4">Name</th>
            <th>Value</th>
            <th class="w-40 text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <!-- Add row -->
          <MetaDataRow
            :key="addRowKey"
            :initial-name="draft.name"
            :initial-value="draft.value"
            :all-names="allNames"
            :is-new="true"
            :is-active="activeIndex === -1"
            :is-editable="uiStore.isTemplateEditable"
            @confirm="createMeta"
            @cancel="resetDraft"
            @delete="resetDraft"
            @row-focus="setActiveIndex(-1)"
          />

          <!-- Small visual gap between add row and existing rows -->
          <tr v-if="customMetaData.length">
            <td colspan="3" class="h-1"></td>
          </tr>

          <!-- Existing rows -->
          <MetaDataRow
            v-for="(meta, index) in customMetaData"
            :key="index"
            :initial-name="meta.name"
            :initial-value="meta.value"
            :all-names="allNames"
            :index="index"
            :is-active="activeIndex === index"
            :is-editable="uiStore.isTemplateEditable"
            @confirm="updateMeta(index, $event)"
            @delete="deleteMeta(index)"
            @row-focus="setActiveIndex(index)"
          />
        </tbody>
      </table>
    </div>
  </div>
</template>
