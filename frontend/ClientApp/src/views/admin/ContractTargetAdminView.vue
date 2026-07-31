<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import MachineCredentialDialog from '@/components/admin/MachineCredentialDialog.vue'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import type { ContractTarget, MachineCredential } from '@/models/responses/contract-response'

/**
 * Administration of the Contract Target Systems deployments may be dispatched
 * to (ADR-25, UC-09-01 system configuration). Deployment used to address a
 * single endpoint from process configuration, so an operator could not say
 * where a contract should go and a failed dispatch had no target to name.
 */

const targets = ref<ContractTarget[]>([])
const loading = ref(false)
const error = ref('')
const saving = ref(false)
const pendingRemovals = ref(new Set<string>())
const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const issued = ref<MachineCredential | null>(null)
const issuedTitle = ref('')
const credentialReturnFocus = ref<HTMLElement | null>(null)

const editingId = ref<string | null>(null)
const form = ref({ name: '', url: '', description: '', enabled: true })

const isEditing = computed(() => editingId.value !== null)

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    targets.value = await contractWorkflowService.listTargets()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not load target systems'
  } finally {
    loading.value = false
  }
}

onMounted(load)

const resetForm = () => {
  editingId.value = null
  form.value = { name: '', url: '', description: '', enabled: true }
}

const edit = (target: ContractTarget) => {
  editingId.value = target.id
  form.value = {
    name: target.name,
    url: target.url,
    description: target.description ?? '',
    enabled: target.enabled,
  }
}

const save = async () => {
  if (saving.value) return

  saving.value = true
  error.value = ''
  try {
    const payload = {
      name: form.value.name.trim(),
      url: form.value.url.trim(),
      description: form.value.description.trim() || undefined,
      enabled: form.value.enabled,
    }
    if (editingId.value) {
      await contractWorkflowService.updateTarget({ ...payload, id: editingId.value })
    } else {
      await contractWorkflowService.createTarget(payload)
    }
    resetForm()
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not save the target system'
  } finally {
    saving.value = false
  }
}

// A target proves it is itself when it acknowledges a deployment, so the
// credential is issued per target rather than shared (ADR-27). Issuing again
// stops the previous secret working.
const issueCredential = async (target: ContractTarget) => {
  credentialReturnFocus.value = document.activeElement instanceof HTMLElement ? document.activeElement : null
  error.value = ''
  try {
    const credential = await contractWorkflowService.rotateTargetSecret(target.id)
    issuedTitle.value = `Callback credential for ${target.name}`
    issued.value = credential
    targets.value = targets.value.map((entry) =>
      entry.id === target.id
        ? {
            ...entry,
            oauth_client_id: credential.client_id,
            secret_issued_at: credential.issued_at ?? entry.secret_issued_at,
          }
        : entry,
    )
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not issue the callback credential'
  }
}

// Removal is refused by the backend while a contract still designates the
// target, so the message it returns is shown rather than swallowed: it names
// how many contracts must be repointed first.
const remove = async (target: ContractTarget) => {
  if (pendingRemovals.value.has(target.id)) return
  const result = await confirmationModal.value?.reveal({
    message: `Remove target system “${target.name}”? Contracts that still designate it must be repointed first.`,
  })
  if (!result || result.isCanceled) return
  pendingRemovals.value = new Set(pendingRemovals.value).add(target.id)
  error.value = ''
  try {
    await contractWorkflowService.deleteTarget(target.id)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not remove the target system'
  } finally {
    const next = new Set(pendingRemovals.value)
    next.delete(target.id)
    pendingRemovals.value = next
  }
}
</script>

<template>
  <div data-testid="target-admin" class="mx-auto flex w-full max-w-7xl min-w-0 flex-col gap-6 p-4 sm:p-6">
    <div>
      <h1 class="text-2xl font-semibold">Target Systems</h1>
      <p class="mt-1 opacity-70">
        Systems a signed contract can be deployed to. Each contract names the one it deploys to, so the deployment that
        follows signing has a destination.
      </p>
    </div>

    <div v-if="error" data-testid="target-admin-error" class="alert rounded-box alert-error" role="alert">
      {{ error }}
    </div>

    <section class="card bg-base-200" aria-labelledby="target-configuration-heading">
      <form class="card-body grid min-w-0 gap-3 md:grid-cols-2" @submit.prevent="save">
        <h2 id="target-configuration-heading" class="card-title md:col-span-2">Target system configuration</h2>
        <p class="text-sm opacity-70 md:col-span-2">
          {{ isEditing ? 'Change the selected deployment destination.' : 'Register a new deployment destination.' }}
        </p>

        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Name</span>
          <input v-model="form.name" data-testid="target-name" required class="input-bordered input w-full min-w-0" />
        </label>

        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Endpoint URL</span>
          <input
            v-model="form.url"
            data-testid="target-url"
            required
            type="url"
            class="input-bordered input w-full min-w-0"
          />
        </label>

        <label class="flex min-w-0 flex-col gap-2 md:col-span-2">
          <span class="label-text">Description</span>
          <input
            v-model="form.description"
            data-testid="target-description"
            class="input-bordered input w-full min-w-0"
          />
        </label>

        <label class="flex cursor-pointer items-center gap-2">
          <input
            v-model="form.enabled"
            data-testid="target-enabled"
            type="checkbox"
            class="checkbox checkbox-sm checkbox-primary"
          />
          <span class="label-text">Accepts deployments</span>
        </label>

        <div class="flex flex-wrap gap-2 md:col-span-2">
          <button
            type="submit"
            data-testid="target-save"
            :disabled="saving"
            :aria-busy="saving"
            class="btn btn-primary"
          >
            <span v-if="saving" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
            <span>{{ saving ? 'Saving…' : isEditing ? 'Save changes' : 'Register' }}</span>
          </button>
          <button v-if="isEditing" type="button" data-testid="target-cancel" class="btn btn-outline" @click="resetForm">
            Cancel
          </button>
        </div>
      </form>
    </section>

    <section class="min-w-0" aria-labelledby="registered-targets-heading">
      <h2 id="registered-targets-heading" class="mb-3 text-lg font-semibold">Registered target systems</h2>
      <div v-if="loading" class="flex items-center gap-2 opacity-70" role="status">
        <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
        Loading…
      </div>
      <div v-else-if="targets.length === 0" data-testid="target-empty-state" class="alert rounded-box alert-info">
        No target system is registered yet. A contract cannot be deployed until one exists and it names it.
      </div>
      <div v-else class="max-w-full overflow-x-auto rounded-box border border-base-300">
        <table class="table min-w-240">
          <thead>
            <tr>
              <th>Name</th>
              <th>Endpoint</th>
              <th>Description</th>
              <th>Deployments</th>
              <th>Callback credential</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="target in targets" :key="target.id" data-testid="target-row">
              <td data-testid="target-row-name">{{ target.name }}</td>
              <td class="font-mono text-xs">{{ target.url }}</td>
              <td>{{ target.description }}</td>
              <td>
                <span v-if="target.enabled" class="badge badge-sm badge-success">accepted</span>
                <span v-else data-testid="target-row-disabled" class="badge badge-sm badge-warning">refused</span>
              </td>
              <td class="text-xs">
                <span
                  v-if="target.oauth_client_id"
                  data-testid="target-row-credentialed"
                  class="badge badge-sm badge-success"
                >
                  issued
                </span>
                <span v-else data-testid="target-row-no-credential" class="badge badge-ghost badge-sm">none</span>
              </td>
              <td>
                <div class="flex flex-wrap gap-2">
                  <button class="btn btn-outline btn-xs" data-testid="target-edit" @click="edit(target)">Edit</button>
                  <button
                    class="btn btn-outline btn-xs"
                    data-testid="target-issue-credential"
                    @click="issueCredential(target)"
                  >
                    {{ target.oauth_client_id ? 'New secret' : 'Issue credential' }}
                  </button>
                  <button
                    class="btn btn-outline btn-xs btn-error"
                    data-testid="target-delete"
                    :disabled="pendingRemovals.has(target.id)"
                    @click="remove(target)"
                  >
                    {{ pendingRemovals.has(target.id) ? 'Removing…' : 'Remove' }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <MachineCredentialDialog
      v-if="issued"
      :credential="issued"
      :title="issuedTitle"
      :return-focus-to="credentialReturnFocus"
      @close="issued = null"
    />
    <ConfirmationModal ref="confirmation-modal" />
  </div>
</template>
