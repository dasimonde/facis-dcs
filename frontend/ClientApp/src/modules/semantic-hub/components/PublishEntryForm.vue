<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { refreshOntologyDomainFields } from '@template-repository/utils/ontology-domain-fields'
import { registerSchema } from '@/services/semantic-hub-service'

/**
 * Publishes a brand-new (name, kind) hub entry from pasted or uploaded raw
 * content — how an operator brings an external vocabulary (a Gaia-X shapes
 * graph, a partner ontology) into the running instance without a rebuild.
 * Registering further versions of an existing entry happens on the entry's
 * own panel (RegisterVersionForm).
 */

// The backend's schema kinds; the clause catalog is the `shapes` entry named
// "clause-catalog", not a kind of its own.
const KINDS = ['context', 'shapes', 'profile', 'ontology'] as const

const MEDIA_TYPE_BY_KIND: Record<(typeof KINDS)[number], string> = {
  context: 'application/ld+json',
  shapes: 'text/turtle',
  profile: 'application/yaml',
  ontology: 'text/turtle',
}

const emit = defineEmits<{
  published: [name: string, kind: string, version: number]
}>()

const name = ref('')
const kind = ref<(typeof KINDS)[number]>('shapes')
const source = ref<'inline' | 'url'>('inline')
const content = ref('')
const sourceUrl = ref('')
const pinnedVersion = ref('')
const activate = ref(true)
const submitting = ref(false)
const error = ref<string | null>(null)

const nameId = useId()
const kindId = useId()
const contentId = useId()
const sourceUrlId = useId()
const versionId = useId()

/**
 * A shape library published on another instance is pinned by version in every
 * template that declares it, and the pin is the publisher's number — so
 * installing it here has to land it at that number, not at this hub's next
 * one. Blank keeps the ordinary behaviour: the next version.
 */
const requestedVersion = computed(() => {
  const parsed = Number.parseInt(pinnedVersion.value.trim(), 10)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
})

const canSubmit = computed(
  () => !!name.value.trim() && (source.value === 'url' ? !!sourceUrl.value.trim() : !!content.value.trim()),
)

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  content.value = await file.text()
  input.value = ''
}

async function submit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  error.value = null
  try {
    const result = await registerSchema({
      name: name.value.trim(),
      kind: kind.value,
      media_type: MEDIA_TYPE_BY_KIND[kind.value],
      ...(source.value === 'url' ? { source_url: sourceUrl.value.trim() } : { content: content.value }),
      ...(requestedVersion.value ? { version: requestedVersion.value } : {}),
      activate: activate.value,
    })
    emit('published', name.value.trim(), kind.value, result.version)
    refreshOntologyDomainFields().catch(() => undefined)
    name.value = ''
    content.value = ''
    sourceUrl.value = ''
    pinnedVersion.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Publishing failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form class="space-y-3" @submit.prevent="submit">
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div class="form-control">
        <label :for="nameId" class="label py-1 text-base-content/70">
          <span class="label-text text-xs">Name</span>
        </label>
        <input
          :id="nameId"
          v-model="name"
          type="text"
          class="input-bordered input input-sm w-full"
          placeholder="e.g. gaiax-participant"
          aria-label="Entry name"
        />
      </div>
      <div class="form-control">
        <label :for="kindId" class="label py-1 text-base-content/70">
          <span class="label-text text-xs">Kind</span>
        </label>
        <select :id="kindId" v-model="kind" class="select-bordered select w-full select-sm" aria-label="Entry kind">
          <option v-for="option in KINDS" :key="option" :value="option">{{ option }}</option>
        </select>
      </div>
    </div>
    <div role="tablist" class="tabs-boxed tabs w-fit tabs-sm">
      <a
        role="tab"
        class="tab"
        :class="[source === 'inline' ? 'tab-active' : 'text-base-content/70']"
        @click="source = 'inline'"
      >
        Paste / upload
      </a>
      <a
        role="tab"
        class="tab"
        :class="[source === 'url' ? 'tab-active' : 'text-base-content/70']"
        @click="source = 'url'"
      >
        From URL
      </a>
    </div>

    <div v-if="source === 'inline'" class="form-control">
      <label :for="contentId" class="label py-1 text-base-content/90">
        <span class="label-text text-xs">Content ({{ MEDIA_TYPE_BY_KIND[kind] }})</span>
        <label class="label-text-alt link cursor-pointer text-xs">
          Upload file
          <input type="file" class="hidden" accept=".ttl,.jsonld,.json,.yaml,.yml,text/*" @change="onFileSelected" />
        </label>
      </label>
      <textarea
        :id="contentId"
        v-model="content"
        class="textarea-bordered textarea h-40 w-full resize-y font-mono text-xs"
        placeholder="Paste the vocabulary's raw content, or upload a file"
        spellcheck="false"
        aria-label="Entry content"
      />
    </div>
    <div v-else class="form-control">
      <label :for="sourceUrlId" class="label py-1 text-base-content/90">
        <span class="label-text text-xs">Source URL</span>
        <span class="label-text-alt text-xs text-base-content/70">follows redirects · snapshotted as a version</span>
      </label>
      <input
        :id="sourceUrlId"
        v-model="sourceUrl"
        type="url"
        class="input-bordered input input-sm w-full font-mono text-xs"
        placeholder="https://w3id.org/gaia-x/development#…"
        aria-label="Schema source URL"
      />
    </div>
    <div class="form-control">
      <label :for="versionId" class="label py-1 text-base-content/70">
        <span class="label-text text-xs">Version (optional)</span>
        <span class="label-text-alt text-xs text-base-content/70">
          installing a library another instance published? use the version its templates pin
        </span>
      </label>
      <input
        :id="versionId"
        v-model="pinnedVersion"
        type="number"
        min="1"
        class="input-bordered input input-sm w-full sm:w-40"
        placeholder="next version"
        aria-label="Register at this version"
      />
    </div>
    <div class="flex items-center justify-between gap-3">
      <label class="label cursor-pointer gap-2 py-0 text-base-content/70">
        <input v-model="activate" type="checkbox" class="checkbox checkbox-sm" />
        <span class="label-text text-xs">Activate immediately</span>
      </label>
      <button type="submit" class="btn btn-sm btn-primary" :disabled="!canSubmit || submitting">
        <span v-if="submitting" class="loading loading-xs loading-spinner" />
        Publish entry
      </button>
    </div>
    <p v-if="error" class="text-xs text-error">{{ error }}</p>
  </form>
</template>
