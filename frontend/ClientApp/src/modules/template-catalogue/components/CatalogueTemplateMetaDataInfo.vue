<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body">
      <h3 class="card-title text-sm">Meta Data</h3>

      <div v-if="customMetaData.length" class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
        <table class="table table-sm">
          <thead>
            <tr>
              <th class="w-56">Name</th>
              <th>Value</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(m, idx) in customMetaData" :key="`${m.name}|${idx}`">
              <td class="font-mono text-xs">{{ m.name }}</td>
              <td class="font-mono text-xs break-all">{{ m.value }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="text-sm text-base-content/60">No meta data available.</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'

const { customMetaData } = storeToRefs(useTemplateDraftStore())
</script>
