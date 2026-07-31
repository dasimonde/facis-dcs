<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import MachineCredentialDialog from '@/components/admin/MachineCredentialDialog.vue'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import type { MachineCredential, MachineIdentity } from '@/models/responses/contract-response'

/**
 * Administration of the machine identities that reach DCS over its API: the SRS
 * Table 5 System Users (ADR-27). Their credentials used to live in the
 * deployment's values file, so adding an integrator or rotating a secret meant
 * editing configuration and redeploying.
 *
 * Sys. Contract Signer is absent on purpose. ADR-17 removes signing scope from
 * that class: eIDAS makes a signatory a natural person under sole control, so a
 * signature always needs a person with a wallet.
 */

const ASSIGNABLE_ROLES = [
  'Sys. Contract Creator',
  'Sys. Contract Reviewer',
  'Sys. Contract Approver',
  'Sys. Contract Manager',
  'Sys. Auditor',
]

const identities = ref<MachineIdentity[]>([])
const loading = ref(false)
const error = ref('')
const saving = ref(false)
const pendingRemovals = ref(new Set<string>())
const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const editingId = ref<string | null>(null)
const form = ref({ name: '', participant_did: '', description: '', roles: [] as string[], enabled: true })

const issued = ref<MachineCredential | null>(null)
const issuedTitle = ref('')
const credentialReturnFocus = ref<HTMLElement | null>(null)

const isEditing = computed(() => editingId.value !== null)

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    identities.value = await contractWorkflowService.listMachineIdentities()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not load system users'
  } finally {
    loading.value = false
  }
}

onMounted(load)

const resetForm = () => {
  editingId.value = null
  form.value = { name: '', participant_did: '', description: '', roles: [], enabled: true }
}

const edit = (identity: MachineIdentity) => {
  editingId.value = identity.id
  form.value = {
    name: identity.name,
    participant_did: identity.participant_did,
    description: identity.description ?? '',
    roles: [...identity.roles],
    enabled: identity.enabled,
  }
}

const save = async () => {
  if (saving.value) return

  credentialReturnFocus.value = document.activeElement instanceof HTMLElement ? document.activeElement : null
  saving.value = true
  error.value = ''
  try {
    const payload = {
      name: form.value.name.trim(),
      participant_did: form.value.participant_did.trim(),
      description: form.value.description.trim() || undefined,
      roles: form.value.roles,
    }
    if (editingId.value) {
      await contractWorkflowService.updateMachineIdentity({
        ...payload,
        id: editingId.value,
        enabled: form.value.enabled,
      })
    } else {
      // Creating one issues its first credential, and this response is the only
      // place the secret appears.
      const created = await contractWorkflowService.createMachineIdentity(payload)
      issuedTitle.value = `Credential for ${created.identity.name}`
      issued.value = created.credential
    }
    resetForm()
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not save the system user'
  } finally {
    saving.value = false
  }
}

const rotate = async (identity: MachineIdentity) => {
  credentialReturnFocus.value = document.activeElement instanceof HTMLElement ? document.activeElement : null
  error.value = ''
  try {
    const credential = await contractWorkflowService.rotateMachineIdentitySecret(identity.id)
    issuedTitle.value = `New credential for ${identity.name}`
    issued.value = credential
    identities.value = identities.value.map((entry) =>
      entry.id === identity.id
        ? {
            ...entry,
            oauth_client_id: credential.client_id,
            secret_issued_at: credential.issued_at ?? entry.secret_issued_at,
          }
        : entry,
    )
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not issue a new secret'
  }
}

const remove = async (identity: MachineIdentity) => {
  if (pendingRemovals.value.has(identity.id)) return
  const result = await confirmationModal.value?.reveal({
    message: `Remove system user “${identity.name}”? Its credentials will stop working and integrations using them will lose access.`,
  })
  if (!result || result.isCanceled) return
  pendingRemovals.value = new Set(pendingRemovals.value).add(identity.id)
  error.value = ''
  try {
    await contractWorkflowService.deleteMachineIdentity(identity.id)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Could not remove the system user'
  } finally {
    const next = new Set(pendingRemovals.value)
    next.delete(identity.id)
    pendingRemovals.value = next
  }
}
</script>

<template>
  <div data-testid="machine-identity-admin" class="mx-auto flex w-full max-w-7xl min-w-0 flex-col gap-6 p-4 sm:p-6">
    <div>
      <h1 class="text-2xl font-semibold">System Users</h1>
      <p class="mt-1 opacity-70">
        Integrations that reach this deployment over its API rather than through a browser. Each authenticates with its
        own credential, so one can be disabled or reissued without touching the others.
      </p>
    </div>

    <div v-if="error" data-testid="machine-identity-error" class="alert rounded-box alert-error" role="alert">
      {{ error }}
    </div>

    <section class="card bg-base-200" aria-labelledby="system-user-configuration-heading">
      <form class="card-body grid min-w-0 gap-3 md:grid-cols-2" @submit.prevent="save">
        <h2 id="system-user-configuration-heading" class="card-title md:col-span-2">System user configuration</h2>
        <p class="text-sm opacity-70 md:col-span-2">
          {{
            isEditing ? 'Change the selected API identity.' : 'Register a new API identity and issue its credential.'
          }}
        </p>

        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Name</span>
          <input v-model="form.name" data-testid="identity-name" required class="input-bordered input w-full min-w-0" />
        </label>

        <label class="flex min-w-0 flex-col gap-2">
          <span class="label-text">Attributed participant DID</span>
          <input
            v-model="form.participant_did"
            data-testid="identity-participant-did"
            required
            class="input-bordered input w-full min-w-0"
          />
          <span class="label-text-alt opacity-70">Its actions appear under this identity in the audit trail.</span>
        </label>

        <label class="flex min-w-0 flex-col gap-2 md:col-span-2">
          <span class="label-text">Description</span>
          <input
            v-model="form.description"
            data-testid="identity-description"
            class="input-bordered input w-full min-w-0"
          />
        </label>

        <fieldset class="md:col-span-2">
          <legend class="label-text mb-1">What it may do</legend>
          <div class="flex flex-wrap gap-3">
            <label v-for="role in ASSIGNABLE_ROLES" :key="role" class="flex cursor-pointer items-center gap-2">
              <input
                v-model="form.roles"
                :value="role"
                type="checkbox"
                class="checkbox checkbox-sm checkbox-primary"
                :data-testid="`identity-role-${role}`"
              />
              <span class="label-text">{{ role }}</span>
            </label>
          </div>
          <p class="mt-2 text-xs opacity-70">
            Signing is not offered: a machine can at most seal, which is a different legal instrument, so a signature
            always needs a person with a wallet.
          </p>
        </fieldset>

        <label v-if="isEditing" class="flex cursor-pointer items-center gap-2">
          <input
            v-model="form.enabled"
            data-testid="identity-enabled"
            type="checkbox"
            class="checkbox checkbox-sm checkbox-primary"
          />
          <span class="label-text">May call this deployment</span>
        </label>

        <div class="flex flex-wrap gap-2 md:col-span-2">
          <button
            type="submit"
            data-testid="identity-save"
            :disabled="saving"
            :aria-busy="saving"
            class="btn btn-primary"
          >
            <span v-if="saving" class="loading loading-sm loading-spinner" aria-hidden="true"></span>
            <span>{{ saving ? 'Saving…' : isEditing ? 'Save changes' : 'Register and issue credential' }}</span>
          </button>
          <button
            v-if="isEditing"
            type="button"
            data-testid="identity-cancel"
            class="btn btn-outline"
            @click="resetForm"
          >
            Cancel
          </button>
        </div>
      </form>
    </section>

    <section class="min-w-0" aria-labelledby="registered-system-users-heading">
      <h2 id="registered-system-users-heading" class="mb-3 text-lg font-semibold">Registered system users</h2>
      <div v-if="loading" class="flex items-center gap-2 opacity-70" role="status">
        <span class="loading loading-sm loading-spinner" aria-hidden="true"></span>
        Loading…
      </div>
      <div v-else-if="identities.length === 0" data-testid="identity-empty-state" class="alert rounded-box alert-info">
        No system user is registered yet.
      </div>
      <div v-else class="max-w-full overflow-x-auto rounded-box border border-base-300">
        <table class="table min-w-240">
          <thead>
            <tr>
              <th>Name</th>
              <th>Client ID</th>
              <th>Roles</th>
              <th>Secret issued</th>
              <th>Status</th>
              <th><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="identity in identities" :key="identity.id" data-testid="identity-row">
              <td data-testid="identity-row-name">{{ identity.name }}</td>
              <td class="font-mono text-xs">{{ identity.oauth_client_id }}</td>
              <td class="text-xs">{{ identity.roles.join(', ') }}</td>
              <td class="text-xs">{{ identity.secret_issued_at ?? '—' }}</td>
              <td>
                <span v-if="identity.enabled" class="badge badge-sm badge-success">enabled</span>
                <span v-else data-testid="identity-row-disabled" class="badge badge-sm badge-warning">disabled</span>
              </td>
              <td>
                <div class="flex flex-wrap gap-2">
                  <button class="btn btn-outline btn-xs" data-testid="identity-edit" @click="edit(identity)">
                    Edit
                  </button>
                  <button class="btn btn-outline btn-xs" data-testid="identity-rotate" @click="rotate(identity)">
                    New secret
                  </button>
                  <button
                    class="btn btn-outline btn-xs btn-error"
                    data-testid="identity-delete"
                    :disabled="pendingRemovals.has(identity.id)"
                    @click="remove(identity)"
                  >
                    {{ pendingRemovals.has(identity.id) ? 'Removing…' : 'Remove' }}
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
