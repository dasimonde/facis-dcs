<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { refreshOntologyDomainFields } from '@template-repository/utils/ontology-domain-fields'
import PublishEntryForm from '@semantic-hub/components/PublishEntryForm.vue'
import RegisterVersionForm from '@semantic-hub/components/RegisterVersionForm.vue'
import {
  getSchemaVersions,
  listSchemas,
  rollbackSchema,
  type SemanticSchemaItem,
  type SemanticSchemaListEntry,
} from '@/services/semantic-hub-service'
import { useClauseCatalogStore } from '@/stores/clause-catalog-store'
import { useErrorStore } from '@/stores/error-store'

/**
 * Semantic Hub management (DCS-FR-TR-03, UC-02-08): the versioned
 * JSON-LD-context / SHACL-shapes / validation-profile store every produced
 * document is validated against (ADR-8/ADR-9). Register a new version of an
 * entry, activate/roll back, and inspect any stored version — activation
 * takes effect immediately for newly produced documents (and, for the
 * clause catalog, in the template builder's palette).
 */
const entries = ref<SemanticSchemaListEntry[]>([])
const loading = ref(false)
const selected = ref<SemanticSchemaListEntry | null>(null)
const versions = ref<SemanticSchemaItem[]>([])
const versionsLoading = ref(false)
const viewedVersion = ref<SemanticSchemaItem | null>(null)
const rollbackInFlight = ref<number | null>(null)

const errorStore = useErrorStore()
const clauseCatalog = useClauseCatalogStore()

const KIND_BADGE: Record<string, string> = {
  context: 'badge-info',
  shapes: 'badge-primary',
  profile: 'badge-secondary',
}

async function loadEntries() {
  loading.value = true
  try {
    entries.value = await listSchemas()
    if (selected.value) {
      const match = entries.value.find((e) => e.name === selected.value?.name && e.kind === selected.value?.kind)
      selected.value = match ?? null
    }
  } finally {
    loading.value = false
  }
}

async function select(entry: SemanticSchemaListEntry) {
  selected.value = entry
  viewedVersion.value = null
  await loadVersions()
}

async function loadVersions() {
  const entry = selected.value
  if (!entry) return
  versionsLoading.value = true
  try {
    versions.value = await getSchemaVersions(entry.name, entry.kind)
  } finally {
    versionsLoading.value = false
  }
}

async function activateVersion(version: number) {
  const entry = selected.value
  if (!entry || rollbackInFlight.value !== null) return
  rollbackInFlight.value = version
  try {
    await rollbackSchema(entry.name, entry.kind, version)
    await afterMutation()
  } catch (err) {
    errorStore.add(err instanceof Error ? err.message : 'Activation failed')
  } finally {
    rollbackInFlight.value = null
  }
}

async function onRegistered() {
  await afterMutation()
}

async function onPublished(name: string, kind: string) {
  await loadEntries()
  const entry = entries.value.find((candidate) => candidate.name === name && candidate.kind === kind)
  if (entry) await select(entry)
}

async function afterMutation() {
  await Promise.all([loadEntries(), loadVersions()])
  // A clause-catalog change must reach the template builder's palette
  // without a reload (ADR-10).
  if (selected.value?.name === 'clause-catalog') void clauseCatalog.refresh()
  // Same for the builder's hub-derived object vocabulary.
  refreshOntologyDomainFields().catch(() => undefined)
}

const activeBadge = computed(() => (v: SemanticSchemaItem) => v.active)

onMounted(loadEntries)
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-4 p-4">
    <header>
      <h1 class="text-xl font-bold">Semantic Hub</h1>
      <p class="mt-1 text-sm text-base-content/70">
        Versioned schema store: JSON-LD contexts, SHACL shapes, and validation profiles every produced document is
        validated against. Activating a version changes what newly produced documents are checked with —
        already-produced documents stay pinned to the version they were created under.
      </p>
    </header>

    <div class="grid gap-4 lg:grid-cols-[minmax(260px,1fr)_2fr]">
      <!-- Entry list -->
      <section class="space-y-4">
        <div class="rounded-lg border border-base-300 bg-base-100 shadow-sm">
          <div class="border-b border-base-300 px-4 py-3">
            <h2 class="text-sm font-semibold text-base-content/80">Entries</h2>
          </div>
          <div v-if="loading && !entries.length" class="p-4 text-sm text-base-content/60">Loading…</div>
          <ul v-else class="divide-y divide-base-200">
            <li v-for="entry in entries" :key="`${entry.kind}/${entry.name}`">
              <button
                type="button"
                class="flex w-full cursor-pointer items-center justify-between gap-2 px-4 py-3 text-left transition-colors hover:bg-base-200/60"
                :class="
                  selected && selected.name === entry.name && selected.kind === entry.kind ? 'bg-base-200/80' : ''
                "
                @click="select(entry)"
              >
                <span class="min-w-0">
                  <span class="block truncate text-sm font-medium">{{ entry.name }}</span>
                  <span class="mt-1 badge badge-xs" :class="KIND_BADGE[entry.kind] ?? 'badge-ghost'">
                    {{ entry.kind }}
                  </span>
                </span>
                <span class="shrink-0 text-right text-xs text-base-content/90">
                  <span v-if="entry.active_version > 0" class="block">active v{{ entry.active_version }}</span>
                  <span v-else class="block text-warning">no active version</span>
                  <span class="block opacity-75">latest v{{ entry.latest_version }}</span>
                </span>
              </button>
            </li>
            <li v-if="!entries.length" class="px-4 py-3 text-sm text-base-content/50">No hub entries.</li>
          </ul>
        </div>

        <!-- Publish new entry -->
        <div class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
          <h2 class="mb-3 text-sm font-semibold text-base-content/80">Publish new entry</h2>
          <PublishEntryForm @published="onPublished" />
        </div>
      </section>

      <!-- Detail -->
      <section v-if="selected" class="space-y-4">
        <div class="rounded-lg border border-base-300 bg-base-100 shadow-sm">
          <div class="flex items-center justify-between border-b border-base-300 px-4 py-3">
            <h2 class="text-sm font-semibold text-base-content/80">
              {{ selected.name }}
              <span class="ml-1 badge badge-xs" :class="KIND_BADGE[selected.kind] ?? 'badge-ghost'">
                {{ selected.kind }}
              </span>
            </h2>
            <span class="text-xs text-base-content/70">{{ selected.media_type }}</span>
          </div>
          <div v-if="versionsLoading" class="p-4 text-sm text-base-content/70">Loading versions…</div>
          <div v-else class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr class="text-base-content/70">
                  <th>Version</th>
                  <th>Status</th>
                  <th>Registered by</th>
                  <th>Registered at</th>
                  <th class="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="v in [...versions].reverse()" :key="v.version">
                  <td class="font-mono">v{{ v.version }}</td>
                  <td>
                    <span v-if="activeBadge(v)" class="badge badge-xs badge-success">active</span>
                  </td>
                  <td class="max-w-40 truncate text-xs">{{ v.created_by }}</td>
                  <td class="text-xs">{{ v.created_at }}</td>
                  <td class="text-right">
                    <button type="button" class="btn btn-ghost btn-xs" @click="viewedVersion = v">View</button>
                    <button
                      v-if="!v.active"
                      type="button"
                      class="btn btn-outline btn-xs"
                      :disabled="rollbackInFlight !== null"
                      @click="activateVersion(v.version)"
                    >
                      <span v-if="rollbackInFlight === v.version" class="loading loading-xs loading-spinner" />
                      Activate
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Content viewer -->
        <div v-if="viewedVersion" class="rounded-lg border border-base-300 bg-base-100 shadow-sm">
          <div class="flex items-center justify-between border-b border-base-300 px-4 py-3">
            <h3 class="text-sm font-semibold text-base-content/80">
              {{ viewedVersion.name }} v{{ viewedVersion.version }}
              <span v-if="viewedVersion.active" class="ml-1 badge badge-xs badge-success">active</span>
            </h3>
            <button type="button" class="btn btn-ghost btn-xs" @click="viewedVersion = null">Close</button>
          </div>
          <pre
            class="max-h-96 overflow-auto p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap"
          ><code>{{ viewedVersion.content }}</code></pre>
        </div>

        <!-- Register new version -->
        <div class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
          <h3 class="mb-3 text-sm font-semibold text-base-content/80">Register new version</h3>
          <RegisterVersionForm
            :name="selected.name"
            :kind="selected.kind"
            :media-type="selected.media_type"
            @registered="onRegistered"
          />
        </div>
      </section>
      <section
        v-else
        class="flex items-center justify-center rounded-lg border border-dashed border-base-300 p-8 text-sm text-base-content/70"
      >
        Select an entry to inspect its versions.
      </section>
    </div>
  </div>
</template>
