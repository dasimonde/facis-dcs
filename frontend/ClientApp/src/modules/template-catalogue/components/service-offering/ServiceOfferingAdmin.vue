<template>
  <section class="card bg-base-100">
    <div class="card-body">
      <div class="mb-4 flex items-center justify-between">
        <h3 class="card-title">
          {{ loading || error ? '' : isUpdateMode ? 'Update Service Offering' : 'Create Service Offering' }}
        </h3>
      </div>

      <div v-if="loading" class="py-4">Loading...</div>
      <div v-else>
        <div
          v-if="error && !currentServiceOffering && !showCreateForm"
          class="alert flex items-start justify-between gap-4 py-4"
        >
          <div class="text-sm">Unable to load service offering data right now.</div>
          <button type="button" class="btn self-start rounded-box btn-sm" @click="loadCurrent">Retry</button>
        </div>
        <div v-else-if="!currentServiceOffering && !showCreateForm" class="py-4">
          <button class="btn rounded-box btn-primary" @click="openCreateForm">Create</button>
        </div>

        <form v-else class="space-y-5" novalidate @submit.prevent="onSubmit">
          <label class="form-control w-full">
            <div class="label">
              <span class="label-text flex items-center gap-2">
                DCS End Point URL
                <span class="text-error">*</span>
              </span>
            </div>
            <input
              v-model="form.end_point_url"
              type="text"
              class="input-bordered input w-full"
              :class="{ validator: showValidation }"
              required
            />
            <div class="validator-hint" :class="{ invisible: !(showValidation && missingEndPointUrl) }">Required</div>
          </label>

          <div class="space-y-3">
            <div class="flex items-center justify-between gap-4">
              <h4 class="text-sm font-bold">Keywords</h4>
              <div class="flex items-center gap-2">
                <input
                  v-model="keywordInput"
                  type="text"
                  class="input-bordered input input-sm w-56"
                  placeholder="Add keyword"
                  @keydown.enter.prevent="addKeyword"
                />
                <button type="button" class="btn btn-sm btn-secondary" :disabled="submitting" @click="addKeyword">
                  Add
                </button>
              </div>
            </div>

            <div v-if="form.keywords.length === 0" class="text-sm text-base-content/60">No keywords added.</div>
            <div v-else class="flex flex-wrap gap-2">
              <div v-for="(kw, index) in form.keywords" :key="`${kw}-${index}`" class="badge badge-lg badge-primary">
                {{ kw }}
                <button
                  type="button"
                  class="btn ml-2 btn-circle btn-ghost btn-xs"
                  aria-label="Remove keyword"
                  :disabled="submitting"
                  @click="removeKeyword(index)"
                >
                  ✕
                </button>
              </div>
            </div>
          </div>

          <label class="form-control w-full">
            <div class="label">
              <span class="label-text">Description</span>
            </div>
            <textarea v-model="form.description" class="textarea-bordered textarea min-h-28 w-full" />
          </label>

          <label class="form-control w-full">
            <div class="label">
              <span class="label-text">Terms and Conditions</span>
            </div>
            <input v-model="form.terms_and_conditions" type="text" class="input-bordered input w-full" />
          </label>

          <!-- no alert bubble: validator-hint is used for field-level errors -->

          <div class="mt-2 card-actions flex flex-col justify-end gap-3 sm:flex-row sm:items-center">
            <button
              v-if="isUpdateMode"
              type="button"
              class="btn rounded-box btn-error"
              :disabled="submitting"
              @click="onDelete"
            >
              Delete
            </button>

            <button class="btn rounded-box btn-primary" :disabled="submitting">
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
import { useServiceOffering } from '@/modules/template-catalogue/composables/useServiceOffering'
import type { TemplateCatalogueCreateServiceOfferingRequest } from '@/models/requests/template-catalogue-integration-request'

const {
  currentServiceOffering,
  loading,
  error,
  loadCurrent,
  createServiceOffering,
  updateServiceOffering,
  deleteServiceOffering,
} = useServiceOffering()

const confirmationModal = ref<InstanceType<typeof ConfirmationModal> | null>(null)

const showCreateForm = ref(false)
const showValidation = ref(false)
const keywordInput = ref('')
const submitting = computed(() => loading.value)

const defaultForm = (): TemplateCatalogueCreateServiceOfferingRequest => ({
  keywords: [],
  description: '',
  end_point_url: '',
  terms_and_conditions: '',
})

const form = ref<TemplateCatalogueCreateServiceOfferingRequest>(defaultForm())

const isUpdateMode = computed(() => !!currentServiceOffering.value)

const missingEndPointUrl = computed(() => !form.value.end_point_url.trim())
const canSubmit = computed(() => !missingEndPointUrl.value && !submitting.value)

watch(
  currentServiceOffering,
  (value) => {
    showValidation.value = false
    if (value) {
      form.value = {
        keywords: [...(value.keywords ?? [])],
        description: value.description ?? '',
        end_point_url: value.end_point_url ?? '',
        terms_and_conditions: value.terms_and_conditions ?? '',
      }
      showCreateForm.value = true
    } else {
      form.value = defaultForm()
      keywordInput.value = ''
      showCreateForm.value = false
    }
  },
  { immediate: true },
)

void loadCurrent()

function openCreateForm() {
  showValidation.value = false
  showCreateForm.value = true
}

function addKeyword() {
  const trimmed = keywordInput.value.trim()
  if (!trimmed) return
  if (form.value.keywords.includes(trimmed)) {
    keywordInput.value = ''
    return
  }
  form.value.keywords.push(trimmed)
  keywordInput.value = ''
}

function removeKeyword(index: number) {
  form.value.keywords.splice(index, 1)
}

async function onSubmit() {
  showValidation.value = true
  if (!canSubmit.value) return
  try {
    if (isUpdateMode.value) {
      await updateServiceOffering(form.value)
    } else {
      await createServiceOffering(form.value)
    }
  } catch (e) {
    console.error('Service offering submit failed:', e)
  }
}

async function onDelete() {
  if (!currentServiceOffering.value) return
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({
      message: 'This will delete the current service offering. This action cannot be undone.',
    })
    if (isCanceled) return
    await deleteServiceOffering()
  } catch (e) {
    console.error('Delete service offering failed:', e)
  }
}
</script>
