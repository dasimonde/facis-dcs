<template>

  <div class="sticky top-0 z-10 shrink-0 bg-base-200 border-b border-base-300">
    <div class="max-w-4xl mx-auto px-6 pt-3">
      <p class="text-xs font-black uppercase tracking-widest text-base-content/40 mb-2">
        {{ title }}
      </p>
      <div role="tablist" class="tabs tabs-lift tabs-lg">
        <a v-for="(tab, _index) in tabs" :key="tab.id" role="tab" class="tab"
          :class="{ 'tab-active': activeTab === tab.id }" @click="setActiveTab(tab.id)">
          {{ tab.label }}
        </a>
      </div>
    </div>
  </div>

  <!-- Tab content -->
  <div class="grow mt-5">
    <div class="max-w-4xl mx-auto p-6">
      <div class="grid grid-cols-1 gap-4">

        <!-- DETAILS TAB -->
        <div v-show="activeTab === 'details'">
          <div class="card bg-base-100 border border-base-300 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title text-sm justify-between">
                <div class="flex gap-2">
                  <span class="badge badge-sm badge-primary">01</span> Template Details
                </div>
                <div v-if="state" class="badge badge-sm badge-secondary">{{ state }}</div>
              </h2>
              <DetailsEditor />
            </div>
          </div>
        </div>

        <!-- SEMANTIC RULES TAB -->
        <div v-show="activeTab === 'semantic'">
          <div class="card bg-base-100 border border-base-300 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title text-sm">
                <span class="badge badge-secondary">02</span> Semantic Rules
              </h2>
              <SemanticRulesEditor />
            </div>
          </div>

        </div>

        <!-- CLAUSES TAB -->
        <div v-show="activeTab === 'clauses'">
          <div class="card bg-base-100 border border-base-300 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title text-sm">
                <span class="badge badge-primary">03</span> Clauses
              </h2>
              <ClausesEditor />
            </div>
          </div>
        </div>

        <!-- BUILDER TAB -->
        <div v-show="activeTab === 'builder'">
          <div class="card bg-base-100 border border-base-300 shadow-sm">
            <div class="card-body">
              <div class="flex items-center justify-between mb-2">
                <h2 class="card-title text-sm">Builder</h2>
                <button type="button" class="btn btn-sm btn-secondary" @click="togglePreviewDialog">
                  Preview
                </button>
              </div>
              <BuilderEditor />
            </div>
          </div>
          <AddBlockModal />
          <BuilderPreviewDialog />
        </div>

        <!-- META TAB -->
        <div v-show="activeTab === 'meta'">
          <div class="card bg-base-100 border border-base-300 shadow-sm">
            <div class="card-body">
              <h2 class="card-title text-sm">Meta Data</h2>
              <MetaDataEditor />
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>

</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore.ts'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import BuilderEditor from '@template-repository/components/BuilderEditor.vue'
import AddBlockModal from '@template-repository/components/builder-editor/AddBlockModal.vue'
import SemanticRulesEditor from '@template-repository/components/SemanticRulesEditor.vue'
import ClausesEditor from '@template-repository/components/ClausesEditor.vue'
import DetailsEditor from '@template-repository/components/DetailsEditor.vue'
import MetaDataEditor from '@template-repository/components/MetaDataEditor.vue'
import BuilderPreviewDialog from '@template-repository/components/builder-editor/BuilderPreviewDialog.vue'
import { storeToRefs } from 'pinia'

const props = withDefaults(
  defineProps<{
    title: string

  }>(),
  {}
)

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useTemplateDraftStore()
const { activeTab } = storeToRefs(templateEditorUiStore)
const { state, templateType } = storeToRefs(draftStore)
const { setActiveTab, togglePreviewDialog } = templateEditorUiStore
const tabs = computed(() => templateEditorUiStore.availableTabs(templateType.value))

</script>
