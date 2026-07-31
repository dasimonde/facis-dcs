<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { registerSchema } from '@/services/semantic-hub-service'

/**
 * Registers a new version of a hub entry from pasted or uploaded raw
 * content (TTL/JSON-LD/YAML). Versions are immutable and monotonic; the
 * activate toggle makes the new version the one newly produced documents
 * anchor to (ADR-8).
 */
const props = defineProps<{
  name: string
  kind: string
  mediaType: string
}>()

const emit = defineEmits<{
  registered: [version: number]
}>()

const content = ref('')
const pinnedVersion = ref('')
const activate = ref(true)
const submitting = ref(false)
const error = ref<string | null>(null)

const contentId = useId()
const versionId = useId()

/**
 * A version of this entry that another instance published carries that
 * instance's number in every template pinning it, so it has to be registered
 * here under the same number. Blank keeps the ordinary behaviour: the next
 * version.
 */
const requestedVersion = computed(() => {
  const parsed = Number.parseInt(pinnedVersion.value.trim(), 10)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
})

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  content.value = await file.text()
  input.value = ''
}

async function submit() {
  if (!content.value.trim() || submitting.value) return
  submitting.value = true
  error.value = null
  try {
    const result = await registerSchema({
      name: props.name,
      kind: props.kind,
      media_type: props.mediaType,
      content: content.value,
      ...(requestedVersion.value ? { version: requestedVersion.value } : {}),
      activate: activate.value,
    })
    content.value = ''
    pinnedVersion.value = ''
    emit('registered', result.version)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Registration failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form class="space-y-3" @submit.prevent="submit">
    <div class="form-control">
      <label :for="contentId" class="label py-1 text-base-content/70">
        <span class="label-text text-xs">New version content ({{ mediaType }})</span>
        <label class="label-text-alt link cursor-pointer text-xs">
          Upload file
          <input type="file" class="hidden" accept=".ttl,.jsonld,.json,.yaml,.yml,text/*" @change="onFileSelected" />
        </label>
      </label>
      <textarea
        :id="contentId"
        v-model="content"
        class="textarea-bordered textarea h-48 w-full resize-y font-mono text-xs"
        placeholder="Paste the new version's raw content, or upload a file"
        spellcheck="false"
      />
    </div>
    <div class="form-control">
      <label :for="versionId" class="label py-1 text-base-content/70">
        <span class="label-text text-xs">Version (optional)</span>
        <span class="label-text-alt text-xs text-base-content/70">
          installing a version another instance published? use the number its templates pin
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
      <button type="submit" class="btn btn-sm btn-primary" :disabled="!content.trim() || submitting">
        <span v-if="submitting" class="loading loading-xs loading-spinner" />
        Register version
      </button>
    </div>
    <p v-if="error" class="text-xs text-error">{{ error }}</p>
  </form>
</template>
