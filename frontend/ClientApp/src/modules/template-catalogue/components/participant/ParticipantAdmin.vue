<template>
  <section class="card bg-base-100">
    <div class="card-body">
      <div class="flex items-center justify-between mb-4">
        <h3 class="card-title">
          {{ loading || error ? '' : (isUpdateMode ? 'Update Participant' : 'Create Participant') }}
        </h3>
      </div>

      <div v-if="loading" class="py-4">Loading...</div>
      <div v-else>
        <div v-if="error && !currentParticipant && !showCreateForm"
          class="alert py-4 flex items-start justify-between gap-4">
          <div class="text-sm">Unable to load participant data right now.</div>
          <button type="button" class="btn btn-sm rounded-box self-start" @click="loadCurrent">Retry</button>
        </div>
        <div v-else-if="!currentParticipant && !showCreateForm" class="py-4">
          <button class="btn btn-primary rounded-box" @click="showValidation = false; showCreateForm = true">
            Create
          </button>
        </div>

        <form v-else class="space-y-5" @submit.prevent="onSubmit" novalidate>
          <fieldset class="space-y-3">
            <legend class="text-sm font-bold">Legal Entity</legend>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text flex items-center gap-2">
                    Legal Name <span class="text-error">*</span>
                  </span>
                </div>
                <input v-model="form.legal_name" type="text" class="input input-bordered w-full"
                  :class="{ validator: showValidation }" required />
                <div class="validator-hint" :class="{ invisible: !(showValidation && missingLegalName) }">
                  Required
                </div>
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text flex items-center gap-2">
                    Registration Number <span class="text-error">*</span>
                  </span>
                </div>
                <input v-model="form.registration_number" type="text" class="input input-bordered w-full"
                  :class="{ validator: showValidation }" required />
                <div class="validator-hint" :class="{ invisible: !(showValidation && missingRegistrationNumber) }">
                  Required
                </div>
              </label>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">LEI Code</span>
                </div>
                <input v-model="form.lei_code" type="text" class="input input-bordered w-full" />
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Ethereum Address</span>
                </div>
                <input v-model="form.ethereum_address" type="text" class="input input-bordered w-full" />
              </label>
            </div>
          </fieldset>

          <fieldset class="space-y-3">
            <legend class="text-sm font-bold">Headquarter Address</legend>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Country</span>
                </div>
                <input v-model="form.headquarter_address.country" type="text" class="input input-bordered w-full" />
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Street Address</span>
                </div>
                <input v-model="form.headquarter_address.street_address" type="text"
                  class="input input-bordered w-full" />
              </label>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Postal Code</span>
                </div>
                <input v-model="form.headquarter_address.postal_code" type="text" class="input input-bordered w-full" />
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Locality</span>
                </div>
                <input v-model="form.headquarter_address.locality" type="text" class="input input-bordered w-full" />
              </label>
            </div>
          </fieldset>

          <fieldset class="space-y-3">
            <legend class="text-sm font-bold">Legal Address</legend>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Country</span>
                </div>
                <input v-model="form.legal_address.country" type="text" class="input input-bordered w-full" />
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Street Address</span>
                </div>
                <input v-model="form.legal_address.street_address" type="text" class="input input-bordered w-full" />
              </label>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Postal Code</span>
                </div>
                <input v-model="form.legal_address.postal_code" type="text" class="input input-bordered w-full" />
              </label>
              <label class="form-control w-full">
                <div class="label">
                  <span class="label-text">Locality</span>
                </div>
                <input v-model="form.legal_address.locality" type="text" class="input input-bordered w-full" />
              </label>
            </div>
          </fieldset>

          <label class="form-control w-full">
            <div class="label">
              <span class="label-text">Terms and Conditions</span>
            </div>
            <input v-model="form.terms_and_conditions" type="text" class="input input-bordered w-full" />
          </label>

          <!-- no alert bubble: validator-hint is used for field-level errors -->

          <div class="card-actions justify-end mt-2 gap-3 flex flex-col sm:flex-row sm:items-center">
            <button v-if="isUpdateMode" type="button" class="btn btn-error rounded-box" :disabled="submitting"
              @click="onDelete">
              Delete
            </button>

            <button class="btn btn-primary rounded-box" :disabled="submitting">
              {{ isUpdateMode ? 'Update' : 'Create' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </section>

  <ConfirmationModal ref="confirmationModal" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { useParticipant } from '@/modules/template-catalogue/composables/useParticipant'
import type { TemplateCatalogueCreateParticipantRequest } from '@/models/requests/template-catalogue-integration-request'

const confirmationModal = ref<InstanceType<typeof ConfirmationModal> | null>(null)

const {
  currentParticipant,
  loading,
  error,
  loadCurrent,
  createParticipant,
  updateParticipant,
  deleteParticipant,
} = useParticipant()

const showCreateForm = ref(false)
const showValidation = ref(false)
const submitting = computed(() => loading.value)

const defaultForm = (): TemplateCatalogueCreateParticipantRequest => ({
  legal_name: '',
  registration_number: '',
  lei_code: '',
  ethereum_address: '',
  headquarter_address: {
    country: '',
    street_address: '',
    postal_code: '',
    locality: '',
  },
  legal_address: {
    country: '',
    street_address: '',
    postal_code: '',
    locality: '',
  },
  terms_and_conditions: '',
})

const form = ref<TemplateCatalogueCreateParticipantRequest>(defaultForm())

const missingLegalName = computed(() => !form.value.legal_name.trim())
const missingRegistrationNumber = computed(() => !form.value.registration_number.trim())

const canSubmit = computed(() => !missingLegalName.value && !missingRegistrationNumber.value && !submitting.value)

const isUpdateMode = computed(() => !!currentParticipant.value)

watch(
  currentParticipant,
  (value) => {
    showValidation.value = false
    if (value) {
      form.value = {
        legal_name: value.legal_name ?? '',
        registration_number: value.registration_number ?? '',
        lei_code: value.lei_code ?? '',
        ethereum_address: value.ethereum_address ?? '',
        headquarter_address: {
          country: value.headquarter_address?.country ?? '',
          street_address: value.headquarter_address?.street_address ?? '',
          postal_code: value.headquarter_address?.postal_code ?? '',
          locality: value.headquarter_address?.locality ?? '',
        },
        legal_address: {
          country: value.legal_address?.country ?? '',
          street_address: value.legal_address?.street_address ?? '',
          postal_code: value.legal_address?.postal_code ?? '',
          locality: value.legal_address?.locality ?? '',
        },
        terms_and_conditions: value.terms_and_conditions ?? '',
      }
      showCreateForm.value = true
    } else {
      form.value = defaultForm()
      showCreateForm.value = false
    }
  },
  { immediate: true },
)

loadCurrent()

async function onSubmit() {
  showValidation.value = true
  if (!canSubmit.value) return
  try {
    if (isUpdateMode.value) {
      await updateParticipant(form.value)
    } else {
      await createParticipant(form.value)
    }
  } catch (e) {
    // errors are surfaced in composable.error
    console.error('Participant submit failed:', e)
  }
}

async function onDelete() {
  if (!currentParticipant.value) return
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({
      message:
        'This will delete the current participant and clear all catalogue data, including the service offering. This action cannot be undone.',
    })
    if (isCanceled) return
    await deleteParticipant()
  } catch (e) {
    console.error('Delete participant failed:', e)
  }
}
</script>
