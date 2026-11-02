<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import {
  type PartialContractTemplate,
  type TemplateComplianceStatus,
  templateComplianceStatuses,
} from '@/models/contract-template/contract-template'
import { contractTemplateService } from '@/services/contract-template-service'
import { type ContractTemplateState, contractTemplateStates } from '@/types/contract-template-state'
import { type PartyType, partyTypes } from '@/types/party-type'
import { toProperCase } from '@/utils/string'
import ListSearch from '../ListSearch.vue'
import type { ContractTemplateSearchResponse } from '@/models/responses/template-response'

defineProps<{
  templates: PartialContractTemplate[]
}>()

const partyType = ref<PartyType | ''>('')
const partyTypeId = useId()
const complianceStatus = ref<TemplateComplianceStatus | ''>('')
const complianceStatusId = useId()
// DCS-IR-TR-07, DCS-IR-TR-08: the dashboard filters the templates by state.
const state = ref<ContractTemplateState | ''>('')
const stateId = useId()
// DCS-IR-TR-07, DCS-IR-TR-08: and by the day the template was created.
const createdFrom = ref('')
const createdFromId = useId()
const createdTo = ref('')
const createdToId = useId()
const filters = computed(() => ({
  party_type: partyType.value || undefined,
  compliance_status: complianceStatus.value || undefined,
  state: state.value || undefined,
  created_from: createdFrom.value || undefined,
  created_to: createdTo.value || undefined,
}))
const emit = defineEmits<{
  searchResult: [value: PartialContractTemplate[] | null]
}>()

const filterLabels: Partial<Record<keyof PartialContractTemplate, string>> = {
  did: 'DID',
  name: 'Name',
  description: 'Description',
  version: 'Version',
  template_data: 'Template Data',
  jurisdiction: 'Jurisdiction',
  industry: 'Industry',
  created_by: 'Owner',
}

const emptyTemplate: PartialContractTemplate = {
  did: '',
  version: 1,
  created_at: '',
  updated_at: '',
  name: '',
  template_type: 'CONTRACT_TEMPLATE',
  party_type: 'SINGLE_PARTY',
  state: 'DRAFT',
  created_by: '',
}

const responseMapper = (response: ContractTemplateSearchResponse) =>
  response.map((item) => ({
    did: item.did,
    name: item.name,
    description: item.description,
    version: parseInt(item.version, 10),
    state: item.state,
    updated_at: item.updated_at,
    created_at: item.created_at,
    template_type: item.template_type,
    party_type: item.party_type,
    created_by: item.created_by,
    jurisdiction: item.jurisdiction,
    industry: item.industry,
    compliance_status: item.compliance_status,
    template_level: item.template_level,
  }))
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center [&>.join]:m-0">
    <div class="shrink-0 text-sm">
      <label :for="partyTypeId" class="sr-only">Party type</label>
      <select :id="partyTypeId" v-model="partyType" class="select w-max min-w-44">
        <option value="">All party types</option>
        <option v-for="option in partyTypes" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
    </div>
    <div class="shrink-0 text-sm">
      <label :for="complianceStatusId" class="sr-only">Compliance status</label>
      <select :id="complianceStatusId" v-model="complianceStatus" class="select w-max min-w-44">
        <option value="">All compliance statuses</option>
        <option v-for="status in templateComplianceStatuses" :key="status" :value="status">
          {{ toProperCase(status.replace('_', ' ')) }}
        </option>
      </select>
    </div>
    <div class="shrink-0 text-sm">
      <label :for="stateId" class="sr-only">State</label>
      <select :id="stateId" v-model="state" class="select w-max min-w-44">
        <option value="">Any state</option>
        <option v-for="option in contractTemplateStates" :key="option" :value="option">
          {{ toProperCase(option) }}
        </option>
      </select>
    </div>
    <div class="shrink-0 text-sm">
      <label :for="createdFromId" class="sr-only">Created from</label>
      <input :id="createdFromId" v-model="createdFrom" type="date" class="input w-max" title="Created from" />
    </div>
    <div class="shrink-0 text-sm">
      <label :for="createdToId" class="sr-only">Created to</label>
      <input :id="createdToId" v-model="createdTo" type="date" class="input w-max" title="Created to" />
    </div>
    <ListSearch
      class="w-full min-w-0 sm:w-auto sm:max-w-full sm:min-w-80 sm:flex-1"
      :items="templates"
      :filter-labels="filterLabels"
      :filters="filters"
      :empty-item="emptyTemplate"
      :search-fn="async (request) => responseMapper(await contractTemplateService.search(request))"
      placeholder="Search templates"
      @search-result="(result) => emit('searchResult', result)"
    />
  </div>
</template>
