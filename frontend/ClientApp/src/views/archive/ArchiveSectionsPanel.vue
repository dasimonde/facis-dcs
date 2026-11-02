<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { type ArchivedContract, type ArchiveSectionItem, archiveService } from '@/services/archive-service'
import { useErrorStore } from '@/stores/error-store'
import { downloadBlob } from '@/utils/download-blob'

/**
 * Per-party sections of an archived contract (DCS-FR-CSA-26). Every reader
 * sees the archived sections assigned to their organization. The Archive
 * Manager also archives a section or clause block of the archived version
 * for the parties that may read it.
 */
const props = defineProps<{ entry: ArchivedContract; canStore: boolean }>()

const errorStore = useErrorStore()
const sections = ref<ArchiveSectionItem[]>([])
const openId = ref<string | null>(null)
const blocks = ref<{ id: string; label: string }[]>([])
const parties = ref<string[]>([])
const sectionId = ref('')
const chosenParties = ref<string[]>([])
const storing = ref(false)

const labels = computed(() => Object.fromEntries(blocks.value.map((block) => [block.id, block.label])))

function sectionLabel(item: ArchiveSectionItem): string {
  const title = item.content?.['dcs:title']
  return labels.value[item.section_id] ?? (typeof title === 'string' && title ? title : item.section_id)
}

function sectionBlocks(node: unknown, found: { id: string; label: string }[] = []) {
  if (Array.isArray(node)) node.forEach((child) => sectionBlocks(child, found))
  else if (node && typeof node === 'object') {
    const block = node as Record<string, unknown>
    if ((block['@type'] === 'dcs:Section' || block['@type'] === 'dcs:Clause') && typeof block['@id'] === 'string') {
      const title = typeof block['dcs:title'] === 'string' && block['dcs:title'] ? block['dcs:title'] : block['@id']
      found.push({ id: block['@id'], label: `${block['@type'] === 'dcs:Section' ? 'Section' : 'Clause'}: ${title}` })
    }
    Object.values(block).forEach((child) => sectionBlocks(child, found))
  }
  return found
}

function partyNames(document: unknown, createdBy: string): string[] {
  const nodes = (document as Record<string, unknown> | undefined)?.['dcs:parties']
  const names = Array.isArray(nodes)
    ? nodes.map((node) => (node as Record<string, unknown>)['dcs:legalName']).filter((n) => typeof n === 'string')
    : []
  return Array.from(new Set([createdBy, ...names].filter((name) => name)))
}

async function loadSections() {
  sections.value = await archiveService.sections(props.entry.did)
}

onMounted(async () => {
  try {
    await loadSections()
    if (!props.canStore) return
    const snapshot = await archiveService.detail(props.entry.did, props.entry.contract_version)
    blocks.value = sectionBlocks(snapshot.contract_data)
    parties.value = partyNames(snapshot.contract_data, snapshot.created_by)
  } catch {
    // the shared http interceptor already surfaced the server's error
  }
})

async function storeSection() {
  if (!sectionId.value || chosenParties.value.length === 0) return
  storing.value = true
  try {
    await archiveService.storeSection(
      props.entry.did,
      props.entry.contract_version,
      sectionId.value,
      chosenParties.value,
    )
    errorStore.add('Section archived.', 'info')
    sectionId.value = ''
    chosenParties.value = []
    await loadSections()
  } catch {
    // the shared http interceptor already surfaced the server's error
  } finally {
    storing.value = false
  }
}

function downloadSection(item: ArchiveSectionItem) {
  const blob = new Blob([JSON.stringify(item.content, null, 2)], { type: 'application/ld+json' })
  downloadBlob(blob, `section-v${item.contract_version}-${item.section_id.replace(/[^\w.-]+/g, '_')}.jsonld`)
}
</script>

<template>
  <div class="space-y-3 p-2 text-sm" :data-testid="`sections-details-${entry.did}`">
    <form
      v-if="canStore"
      class="flex flex-wrap items-end gap-3"
      data-testid="archive-section-store"
      @submit.prevent="storeSection"
    >
      <label class="fieldset min-w-0">
        Section or clause
        <select v-model="sectionId" class="select select-sm" data-testid="archive-section-block">
          <option value="" disabled>Select a block</option>
          <option v-for="block in blocks" :key="block.id" :value="block.id">{{ block.label }}</option>
        </select>
      </label>
      <fieldset class="fieldset min-w-0">
        <legend>Parties that may read it</legend>
        <label v-for="party in parties" :key="party" class="flex items-center gap-2">
          <input
            v-model="chosenParties"
            type="checkbox"
            class="checkbox checkbox-sm"
            :value="party"
            :data-testid="`archive-section-party-${party}`"
          />
          <span class="wrap-anywhere">{{ party }}</span>
        </label>
      </fieldset>
      <button
        type="submit"
        class="btn btn-sm btn-primary"
        :disabled="storing || !sectionId || chosenParties.length === 0"
        data-testid="archive-section-submit"
      >
        Archive section
      </button>
    </form>

    <p v-if="sections.length === 0" class="opacity-70">No archived sections you may read.</p>
    <table v-else class="table table-xs">
      <thead>
        <tr>
          <th>Section</th>
          <th>Parties</th>
          <th>Stored</th>
          <th>Hash</th>
          <th><span class="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        <template v-for="item in sections" :key="item.id">
          <tr :data-testid="`archive-section-${item.section_id}`">
            <td class="wrap-anywhere">
              {{ sectionLabel(item) }}
              <div class="text-xs opacity-70">Version {{ item.contract_version }}</div>
            </td>
            <td class="wrap-anywhere">{{ item.parties.join(', ') }}</td>
            <td>{{ new Date(item.stored_at).toLocaleString() }} by {{ item.stored_by }}</td>
            <td class="font-mono text-xs wrap-anywhere">{{ item.content_hash }}</td>
            <td>
              <template v-if="item.content">
                <button
                  type="button"
                  class="btn btn-ghost btn-xs"
                  :aria-expanded="openId === item.id"
                  @click="openId = openId === item.id ? null : item.id"
                >
                  Open
                </button>
                <button type="button" class="btn btn-ghost btn-xs" @click="downloadSection(item)">Download</button>
              </template>
              <span v-else class="text-xs opacity-70">Content erased</span>
            </td>
          </tr>
          <tr v-if="openId === item.id">
            <td colspan="5">
              <pre
                class="max-h-80 overflow-auto rounded-box bg-base-200 p-2 text-xs"
                :data-testid="`archive-section-content-${item.section_id}`"
                >{{ JSON.stringify(item.content, null, 2) }}</pre
              >
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
