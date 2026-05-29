<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body">
      <h3 class="card-title text-sm">Details</h3>

      <div class="grid grid-cols-1 gap-4">
        <fieldset class="fieldset border-none p-0">
          <legend class="fieldset-legend">Version</legend>
          <input :value="version ?? '—'" class="input-bordered input w-full" type="text" disabled readonly />
        </fieldset>

        <fieldset class="fieldset border-none p-0">
          <legend class="fieldset-legend">Contract Type</legend>
          <div class="mt-1 grid grid-cols-2 gap-3">
            <div
              class="pointer-events-none card border-2 transition-all"
              :class="templateType === TemplateType.frameContract ? 'border-primary bg-primary/5' : 'border-base-300'"
            >
              <div class="card-body gap-1 p-4">
                <span class="card-title text-sm">Frame Contract</span>
                <p class="text-xs font-normal text-base-content/60">Top-level agreement that groups subcontracts</p>
              </div>
            </div>
            <div
              class="pointer-events-none card border-2 transition-all"
              :class="templateType === TemplateType.subContract ? 'border-primary bg-primary/5' : 'border-base-300'"
            >
              <div class="card-body gap-1 p-4">
                <span class="card-title text-sm">Subcontract</span>
                <p class="text-xs font-normal text-base-content/60">Scoped agreement under a frame contract</p>
              </div>
            </div>
          </div>
        </fieldset>

        <fieldset class="fieldset border-none p-0">
          <legend class="fieldset-legend">Global Name</legend>
          <input :value="name" class="input-bordered input w-full" type="text" disabled readonly />
        </fieldset>

        <fieldset class="fieldset border-none p-0">
          <legend class="fieldset-legend">Base Description</legend>
          <textarea :value="description" class="textarea-bordered textarea h-24 w-full" disabled readonly />
        </fieldset>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { TemplateType } from '@template-repository/models/contract-templace'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'

const { templateType, name, description, version } = storeToRefs(useTemplateDraftStore())
</script>
