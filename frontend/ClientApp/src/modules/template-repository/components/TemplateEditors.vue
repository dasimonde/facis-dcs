<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AddBlockModal from '@template-repository/components/builder-editor/AddBlockModal.vue'
import BuilderPreviewDialog from '@template-repository/components/builder-editor/BuilderPreviewDialog.vue'
import BuilderEditor from '@template-repository/components/BuilderEditor.vue'
import ClauseEditor from '@template-repository/components/clauses-editor/ClauseEditor.vue'
import ClausesEditor from '@template-repository/components/ClausesEditor.vue'
import DataObjectsEditor from '@template-repository/components/data-objects/DataObjectsEditor.vue'
import DetailsEditor from '@template-repository/components/DetailsEditor.vue'
import MetaDataEditor from '@template-repository/components/MetaDataEditor.vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import AuditView from './AuditView.vue'

withDefaults(
  defineProps<{
    title: string
    nameError?: string
    descriptionError?: string
  }>(),
  {},
)

const route = useRoute()

const templateEditorUiStore = useTemplateEditorUiStore()
const draftStore = useDcsDraftStore()
const { activeTab } = storeToRefs(templateEditorUiStore)
const { state, templateType } = storeToRefs(draftStore)
const tabs = computed(() => {
  return templateEditorUiStore.availableTabs(templateType.value).filter((tab) => {
    return tab.id !== 'audit' || !!route.params.did
  })
})
const currentTabNumber = computed(() => 1 + tabs.value.map((tab) => tab.id).indexOf(activeTab.value))
const { isManager } = useTemplatePermissions()
</script>

<template>
  <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
    <div class="mx-auto max-w-5xl px-6 pt-3">
      <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">
        {{ title }}
      </p>
      <div role="tablist" class="tabs-border tabs tabs-lg">
        <a
          v-for="(tab, _index) in tabs"
          :key="tab.id"
          role="tab"
          class="tab text-base-content/70"
          :class="{ 'tab-active text-primary': activeTab === tab.id }"
          @click="templateEditorUiStore.setActiveTab(tab.id)"
        >
          {{ tab.label }}
        </a>
      </div>
    </div>
  </div>

  <!-- Tab content -->
  <div class="mt-5 grow">
    <div class="mx-auto max-w-5xl p-6">
      <div class="grid grid-cols-1 gap-4">
        <slot name="before-tabs" />
        <!-- DETAILS TAB -->
        <div v-show="activeTab === 'details'">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title justify-between text-sm">
                <div class="flex gap-2">
                  <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                  Template Details
                </div>
                <div v-if="state" class="badge badge-sm badge-secondary">{{ state }}</div>
              </h2>
              <DetailsEditor :name-error="nameError" :description-error="descriptionError" />
            </div>
          </div>
        </div>

        <!-- CLAUSES TAB -->
        <div v-show="activeTab === 'clauses'">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title text-sm">
                <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                Clauses
              </h2>
              <ClauseEditor />
              <div class="divider text-xs text-base-content/70">existing clauses</div>
              <ClausesEditor />
            </div>
          </div>
        </div>

        <!-- BUILDER TAB -->
        <div v-show="activeTab === 'builder'">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body">
              <div class="mb-2 flex items-start justify-between">
                <h2 class="card-title text-sm">
                  <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                  Builder
                </h2>
                <button
                  type="button"
                  class="btn btn-sm btn-secondary"
                  @click="templateEditorUiStore.togglePreviewDialog"
                >
                  Preview
                </button>
              </div>
              <BuilderEditor />
            </div>
          </div>
          <AddBlockModal />
          <BuilderPreviewDialog />
        </div>

        <!-- DATA TAB -->
        <div v-show="activeTab === 'data'">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body gap-5">
              <h2 class="card-title text-sm">
                <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                Semantic Data Objects
              </h2>
              <DataObjectsEditor mode="template" :editable="templateEditorUiStore.isTemplateEditable" />
            </div>
          </div>
        </div>

        <!-- META TAB -->
        <div v-show="activeTab === 'meta'">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body">
              <h2 class="card-title text-sm">
                <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                Meta Data
              </h2>
              <MetaDataEditor />
            </div>
          </div>
        </div>

        <template v-if="isManager">
          <div v-show="activeTab === 'audit'">
            <div class="card border border-base-300 bg-base-100 shadow-sm">
              <div class="card-body">
                <h2 class="card-title text-sm">
                  <span class="badge w-8 badge-sm badge-primary">0{{ currentTabNumber }}</span>
                  Audit History
                </h2>
                <AuditView />
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
