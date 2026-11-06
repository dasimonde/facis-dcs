<script setup lang="ts">
import { ref } from 'vue'
import { auditingService } from '@/services/auditing-service'
import { verifyMerkleInclusion } from '@/utils/merkle-inclusion'
import { claimReportedHttpError } from '@/utils/report-action-error'
import type { CheckpointProof } from '@/models/responses/auditing-response'

/**
 * The Auditor proves that one audit entry is part of a timestamped checkpoint
 * (DCS-FR-PACM-01, ADR-16). The browser verifies the proof against the root
 * of the checkpoint; the root to trust is the one of the external anchor.
 */
const entryCid = ref('')
const proof = ref<CheckpointProof | null>(null)
const verified = ref<boolean | null>(null)
const loading = ref(false)
const error = ref('')

async function loadProof() {
  const cid = entryCid.value.trim()
  if (!cid) return
  loading.value = true
  error.value = ''
  proof.value = null
  verified.value = null
  try {
    const result = await auditingService.checkpointProof(cid)
    proof.value = result
    verified.value = await verifyMerkleInclusion(
      result.leaf_hash,
      result.siblings,
      result.leaf_index,
      result.head.leaf_count,
      result.head.root,
    )
  } catch (err) {
    claimReportedHttpError(err)
    error.value = err instanceof Error ? err.message : 'The inclusion proof could not be loaded.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="rounded-box border border-base-content/10 bg-base-100 p-4" data-testid="checkpoint-proof">
    <h3 class="font-bold">Checkpoint inclusion proof</h3>
    <form class="mt-2 flex flex-wrap items-end gap-2" @submit.prevent="loadProof">
      <label class="form-control grow">
        <span class="label-text mb-1">Audit entry CID</span>
        <input v-model="entryCid" class="input-bordered input input-sm" data-testid="checkpoint-proof-cid" />
      </label>
      <button type="submit" class="btn btn-sm" :disabled="loading || !entryCid.trim()">Verify inclusion</button>
    </form>
    <p v-if="error" class="mt-2 text-sm text-error">{{ error }}</p>
    <div v-if="proof" class="mt-3 space-y-2 text-sm wrap-anywhere" data-testid="checkpoint-proof-result">
      <span class="badge" :class="verified ? 'badge-success' : 'badge-error'" data-testid="checkpoint-proof-verdict">
        {{ verified ? 'Proof verified against the checkpoint root' : 'Proof does not match the checkpoint root' }}
      </span>
      <dl class="grid grid-cols-[10rem_minmax(0,1fr)] gap-x-3 gap-y-1">
        <dt class="opacity-70">Checkpoint</dt>
        <dd>
          {{ proof.head.seq }} · {{ proof.head.leaf_count }} entries ·
          {{ new Date(proof.head.created_at).toLocaleString() }}
          <span v-if="proof.head.timestamped_at">
            · timestamped {{ new Date(proof.head.timestamped_at).toLocaleString() }}
          </span>
        </dd>
        <dt class="opacity-70">Root</dt>
        <dd class="font-mono text-xs">{{ proof.head.root }}</dd>
        <dt class="opacity-70">Leaf</dt>
        <dd class="font-mono text-xs">{{ proof.leaf_index }} · {{ proof.leaf_hash }}</dd>
        <dt class="opacity-70">Siblings</dt>
        <dd class="font-mono text-xs">
          <div v-for="sibling in proof.siblings" :key="sibling">{{ sibling }}</div>
        </dd>
      </dl>
    </div>
  </section>
</template>
