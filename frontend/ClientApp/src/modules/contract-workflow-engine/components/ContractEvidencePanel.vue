<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, useTemplateRef, watch } from 'vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { isEditableContractState } from '@/types/contract-state'
import { downloadBlob } from '@/utils/download-blob'
import type { Contract } from '@/models/contract/contract'
import type { ContractEvidence } from '@/models/contract/contract-evidence'

const props = defineProps<{ contract: Contract }>()
const auth = useAuthStore()
// The creator adds evidence while the contract is a draft (DCS-IR-CWE-02); the
// manager stores evidence in every lifecycle state (DCS-IR-CWE-12).
const canStore = computed(
  () =>
    auth.user?.roles.some(
      (role) =>
        role === 'CONTRACT_MANAGER' || (role === 'CONTRACT_CREATOR' && isEditableContractState(props.contract.state)),
    ) ?? false,
)
const entries = ref<ContractEvidence[]>([])
const loading = ref(false)
const pending = ref(false)
const error = ref('')
const storeError = ref('')
const downloadError = ref('')
const downloading = ref<string | null>(null)
const editing = ref(false)
const description = ref('')
const file = ref<File | null>(null)
const fileInput = useTemplateRef<HTMLInputElement>('file-input')
const descriptionInput = useTemplateRef<HTMLTextAreaElement>('description-input')
const openButton = useTemplateRef<HTMLButtonElement>('open-button')
const fileId = useId()
const descriptionId = useId()
let generation = 0
let listRequest = 0
onBeforeUnmount(() => {
  generation++
  listRequest++
})

async function load() {
  const request = ++listRequest
  entries.value = []
  error.value = ''
  loading.value = true
  try {
    const result = await contractWorkflowService.listEvidence(props.contract.did)
    if (request === listRequest) entries.value = result
  } catch (err) {
    if (request === listRequest) error.value = err instanceof Error ? err.message : 'Evidence could not be loaded.'
  } finally {
    if (request === listRequest) loading.value = false
  }
}
watch(
  () => props.contract.did,
  () => {
    generation++
    editing.value = false
    pending.value = false
    file.value = null
    description.value = ''
    storeError.value = ''
    downloadError.value = ''
    downloading.value = null
    void load()
  },
  { immediate: true },
)
async function open() {
  editing.value = true
  storeError.value = ''
  await nextTick()
  fileInput.value?.focus()
}
async function cancel() {
  if (pending.value) return
  editing.value = false
  file.value = null
  description.value = ''
  storeError.value = ''
  await nextTick()
  openButton.value?.focus()
}
function chooseFile(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0] ?? null
}
function readFile(value: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error('The selected file could not be read.'))
    reader.onload = () => {
      if (typeof reader.result !== 'string') {
        reject(new Error('The selected file could not be read.'))
        return
      }
      resolve(reader.result.slice(reader.result.indexOf(',') + 1))
    }
    reader.readAsDataURL(value)
  })
}
async function store() {
  if (pending.value || !canStore.value) return
  storeError.value = ''
  if (!file.value || file.value.size === 0) {
    storeError.value = 'Choose a non-empty evidence file.'
    fileInput.value?.focus()
    return
  }
  if (!description.value.trim()) {
    storeError.value = 'Describe the evidence.'
    descriptionInput.value?.focus()
    return
  }
  const current = generation
  const selected = file.value
  const contract = props.contract
  pending.value = true
  try {
    const content = await readFile(selected)
    if (current !== generation) return
    await contractWorkflowService.storeEvidence({
      did: contract.did,
      updated_at: contract.updated_at,
      filename: selected.name,
      media_type: selected.type || undefined,
      description: description.value.trim(),
      content,
    })
    if (current !== generation) return
    pending.value = false
    await cancel()
    await load()
  } catch (err) {
    if (current === generation) storeError.value = err instanceof Error ? err.message : 'Evidence could not be stored.'
  } finally {
    if (current === generation) pending.value = false
  }
}
async function download(entry: ContractEvidence) {
  if (downloading.value) return
  const current = generation
  downloading.value = entry.evidence_id
  downloadError.value = ''
  try {
    const result = await contractWorkflowService.retrieveEvidence(props.contract.did, entry.evidence_id)
    if (current !== generation) return
    if (!result.content || result.did !== props.contract.did || result.evidence_id !== entry.evidence_id)
      throw new Error('Evidence content is unavailable.')
    const bytes = Uint8Array.from(atob(result.content), (char) => char.charCodeAt(0))
    downloadBlob(new Blob([bytes], { type: result.media_type }), result.filename)
  } catch (err) {
    if (current === generation)
      downloadError.value = err instanceof Error ? err.message : 'Evidence could not be downloaded.'
  } finally {
    if (current === generation) downloading.value = null
  }
}
</script>
<template>
  <section class="mb-6 space-y-3 rounded-box border border-base-300 p-4" aria-label="Contract evidence">
    <h3 class="font-semibold">Contract evidence</h3>
    <button v-if="canStore && !editing" ref="open-button" class="btn btn-sm btn-primary" @click="open">
      Store evidence
    </button>
    <form v-if="canStore && editing" class="space-y-3" novalidate @submit.prevent="store" @keydown.esc.prevent="cancel">
      <label :for="fileId" class="block">Evidence file</label>
      <input
        :id="fileId"
        ref="file-input"
        type="file"
        class="file-input w-full"
        required
        :disabled="pending"
        @change="chooseFile"
      />
      <label :for="descriptionId" class="block">Evidence description</label>
      <textarea
        :id="descriptionId"
        ref="description-input"
        v-model="description"
        class="textarea w-full"
        required
        :disabled="pending"
      />
      <p v-if="storeError" role="alert" class="text-error">{{ storeError }}</p>
      <div class="flex flex-wrap gap-2">
        <button type="submit" class="btn btn-primary" :disabled="pending">
          {{ pending ? 'Storing evidence…' : 'Save evidence' }}
        </button>
        <button type="button" class="btn" :disabled="pending" @click="cancel">Cancel</button>
      </div>
    </form>
    <p v-if="loading" role="status">Loading evidence…</p>
    <div v-else-if="error" role="alert">
      {{ error }}
      <button class="btn btn-sm" @click="load">Retry evidence</button>
    </div>
    <p v-else-if="!entries.length">No stored evidence.</p>
    <ul v-else class="space-y-3">
      <li v-for="entry in entries" :key="entry.evidence_id" class="space-y-1 rounded-box bg-base-200 p-3 break-words">
        <p class="font-semibold">{{ entry.filename }}</p>
        <p>{{ entry.description }}</p>
        <p class="text-sm">Version {{ entry.contract_version }} · {{ entry.recorded_at }} · {{ entry.recorded_by }}</p>
        <p class="text-xs break-all">SHA-256: {{ entry.sha256 }}</p>
        <button
          class="btn btn-sm"
          :disabled="!!downloading"
          :aria-label="`Download ${entry.filename}`"
          @click="download(entry)"
        >
          Download
        </button>
      </li>
    </ul>
    <p v-if="downloadError" role="alert" class="text-error">{{ downloadError }}</p>
  </section>
</template>
