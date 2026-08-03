<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, useId } from 'vue'
import { TemplateType } from '@template-repository/models/contract-template'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templateType, version } = storeToRefs(store)

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim()),
})
const nameId = useId()

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value),
})
const descriptionId = useId()
const props = defineProps<{
  nameError?: string
  descriptionError?: string
}>()
const nameErrorId = useId()
const descriptionErrorId = useId()
</script>

<template>
  <div class="grid grid-cols-1 gap-4">
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Version: {{ version }}</legend>
    </fieldset>
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Contract Type</legend>
      <div class="mt-1 grid grid-cols-2 gap-3">
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.contractTemplate ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Contract</span>
            <p class="text-xs font-normal text-base-content/70">Top-level contract template that can serve as parent</p>
          </div>
        </div>
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.component ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Component</span>
            <p class="text-xs font-normal text-base-content/70">
              Reusable partial contract, embeddable in other templates
            </p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Global Name</legend>
      <label :for="nameId" class="sr-only">Global Name</label>
      <input
        :id="nameId"
        v-model="name"
        class="input-bordered input w-full"
        data-testid="template-global-name"
        type="text"
        required
        :aria-invalid="!!props.nameError"
        :aria-describedby="props.nameError ? nameErrorId : undefined"
        :disabled="!uiStore.isTemplateEditable"
      />
      <p v-if="props.nameError" :id="nameErrorId" class="text-sm text-error">{{ props.nameError }}</p>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Base Description</legend>
      <label :for="descriptionId" class="sr-only">Base Description</label>
      <textarea
        :id="descriptionId"
        v-model="description"
        class="textarea-bordered textarea h-24 w-full"
        data-testid="template-base-description"
        required
        :aria-invalid="!!props.descriptionError"
        :aria-describedby="props.descriptionError ? descriptionErrorId : undefined"
        :disabled="!uiStore.isTemplateEditable"
      ></textarea>
      <p v-if="props.descriptionError" :id="descriptionErrorId" class="text-sm text-error">
        {{ props.descriptionError }}
      </p>
    </fieldset>
  </div>
</template>
