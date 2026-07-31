<script setup lang="ts">
import { computed } from 'vue'
import { fieldFillScalar } from '@/models/dcs-jsonld'
import type { Contract } from '@/models/contract/contract'
import type { DcsContractField } from '@/models/dcs-jsonld'

/**
 * The machine-readable side of the contract, beside the human-readable document
 * (SRS UC-04 stimulus 5: a Creator, Reviewer or Manager "requests a view of the
 * contract in both machine-readable and human-readable formats", and the system
 * "renders synchronized representations of both views"; DCS-FR-CWE-04 requires
 * both to derive from the same source).
 *
 * Both views here come from the same contract_data the document is rendered
 * from, so they cannot drift: this is a second presentation of one payload, not
 * a second copy of it.
 *
 * Collapsed by default and deliberately quiet. It is for whoever needs the node
 * IRIs — the identifiers a KPI reports against and an ODRL constraint binds to,
 * which are otherwise invisible to anyone reading the contract.
 */

const props = defineProps<{ contract: Contract }>()

const fields = computed<DcsContractField[]>(() => props.contract.contract_data?.['dcs:contractFields'] ?? [])

const payload = computed(() => JSON.stringify(props.contract.contract_data ?? {}, null, 2))

const valueOf = (field: DcsContractField) => {
  const value = fieldFillScalar(field['dcs:value'])
  return value === undefined || value === null || String(value).trim() === '' ? '—' : String(value)
}
</script>

<template>
  <details data-testid="machine-readable-view" class="collapse-arrow collapse border border-base-300 bg-base-100">
    <summary data-testid="machine-readable-toggle" class="collapse-title text-sm font-medium opacity-70">
      Machine-readable view
    </summary>

    <div class="collapse-content flex flex-col gap-4 text-xs">
      <p class="opacity-60">
        The same content the document above is rendered from, as the system stores it. Node identifiers are what
        automated checks bind to: a reported KPI names the field by its identifier, not by its label.
      </p>

      <div v-if="fields.length" class="overflow-x-auto">
        <table class="table table-xs">
          <thead>
            <tr>
              <th>Field</th>
              <th>Value</th>
              <th>Identifier</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="field in fields" :key="field['@id']" data-testid="machine-readable-field">
              <td data-testid="machine-readable-field-label">{{ field['dcs:label'] }}</td>
              <td>{{ valueOf(field) }}</td>
              <td data-testid="machine-readable-field-id" class="font-mono break-all opacity-60">
                {{ field['@id'] }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div>
        <div class="mb-1 opacity-60">Contract payload (JSON-LD)</div>
        <pre
          data-testid="machine-readable-payload"
          class="max-h-96 overflow-auto rounded-box bg-base-200 p-3 font-mono break-all whitespace-pre-wrap"
          >{{ payload }}</pre
        >
      </div>
    </div>
  </details>
</template>
