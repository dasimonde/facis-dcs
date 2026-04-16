<template>
  <div class="flex justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>

  <div class="max-w-4xl mx-auto">
    <div role="tablist" class="tabs tabs-lift tabs-lg">
      <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'participant' }"
        @click="activeTab = 'participant'">
        Participant
      </a>
      <a role="tab" class="tab" :class="{ 'tab-active': activeTab === 'serviceOffering' }"
        @click="activeTab = 'serviceOffering'">
        Service Offering
      </a>
    </div>

    <div class="mt-6">
      <ParticipantAdmin v-if="activeTab === 'participant'" />
      <ServiceOfferingAdmin v-else />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import ParticipantAdmin from '@/modules/template-catalogue/components/participant/ParticipantAdmin.vue'
import ServiceOfferingAdmin from '@/modules/template-catalogue/components/service-offering/ServiceOfferingAdmin.vue'

const activeTab = ref<'participant' | 'serviceOffering'>('participant')
</script>
