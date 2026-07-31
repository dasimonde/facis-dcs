<script setup lang="ts">
import { TemplateType, type TemplateTypeValue } from '@template-repository/models/contract-template'

defineProps<{
  modelValue: TemplateTypeValue
}>()

const emit = defineEmits<{
  'update:modelValue': [value: TemplateTypeValue]
}>()
</script>

<template>
  <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
    <button
      type="button"
      class="flex min-h-50 flex-col justify-center rounded-2xl border-2 p-8 text-left transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
      :class="
        modelValue === TemplateType.contractTemplate
          ? 'border-primary bg-primary/10 shadow-lg'
          : 'border-base-300 hover:border-primary/50 hover:bg-base-200/50'
      "
      @click="emit('update:modelValue', TemplateType.contractTemplate)"
    >
      <h2 class="mb-3 text-xl font-bold text-base-content">Contract</h2>
      <p class="text-sm leading-relaxed text-base-content/70">
        A contract template composed of one or more components. Contracts created from this template can serve as the
        parent for other contracts.
      </p>
    </button>
    <button
      type="button"
      class="flex min-h-50 flex-col justify-center rounded-2xl border-2 p-8 text-left transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
      :class="
        modelValue === TemplateType.component
          ? 'border-primary bg-primary/10 shadow-lg'
          : 'border-base-300 hover:border-primary/50 hover:bg-base-200/50'
      "
      @click="emit('update:modelValue', TemplateType.component)"
    >
      <h2 class="mb-3 text-xl font-bold text-base-content">Component</h2>
      <p class="text-sm leading-relaxed text-base-content/70">
        A reusable partial contract template. Components can be embedded in other contract templates.
      </p>
    </button>
  </div>
</template>
