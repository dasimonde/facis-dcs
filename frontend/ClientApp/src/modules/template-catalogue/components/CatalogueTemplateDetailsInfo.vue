<template>
  <div class="card bg-base-100 border border-base-300 shadow-sm">
    <div class="card-body">
      <h3 class="card-title text-sm">Details</h3>

      <div class="grid grid-cols-1 gap-4">
        <fieldset class="fieldset p-0 border-none">
          <legend class="fieldset-legend">Version</legend>
          <input :value="version ?? '—'" class="input input-bordered w-full" type="text" disabled readonly />
        </fieldset>

        <fieldset class="fieldset p-0 border-none">
          <legend class="fieldset-legend">Contract Type</legend>
          <div class="grid grid-cols-2 gap-3 mt-1">
            <div
              class="card border-2 transition-all pointer-events-none"
              :class="templateType === TemplateType.frameContract
                ? 'border-primary bg-primary/5'
                : 'border-base-300'"
            >
              <div class="card-body p-4 gap-1">
                <span class="card-title text-sm">Frame Contract</span>
                <p class="text-xs text-base-content/60 font-normal">
                  Top-level agreement that groups subcontracts
                </p>
              </div>
            </div>
            <div
              class="card border-2 transition-all pointer-events-none"
              :class="templateType === TemplateType.subContract
                ? 'border-primary bg-primary/5'
                : 'border-base-300'"
            >
              <div class="card-body p-4 gap-1">
                <span class="card-title text-sm">Subcontract</span>
                <p class="text-xs text-base-content/60 font-normal">
                  Scoped agreement under a frame contract
                </p>
              </div>
            </div>
          </div>
        </fieldset>

        <fieldset class="fieldset p-0 border-none">
          <legend class="fieldset-legend">Global Name</legend>
          <input :value="name" class="input input-bordered w-full" type="text" disabled readonly />
        </fieldset>

        <fieldset class="fieldset p-0 border-none">
          <legend class="fieldset-legend">Base Description</legend>
          <textarea
            :value="description"
            class="textarea textarea-bordered w-full h-24"
            disabled
            readonly
          />
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
