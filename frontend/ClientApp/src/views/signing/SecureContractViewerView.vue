<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import SigningCeremonyDialog from '@/components/signing/SigningCeremonyDialog.vue'
import { ERASED_CONTENT_MESSAGE, isErasedContentMessage } from '@/composables/useDocumentExport'
import { ROUTES } from '@/router/router'
import { getLocalDIDFile } from '@/services/did-service'
import {
  type ProvenanceEntry,
  type SignatureContract,
  type SignatureEnvelope,
  signatureManagementService,
  type SignatureValidateResult,
  type SignatureVerifyResult,
  signingErrorMessage,
} from '@/services/signature-management-service'
import { downloadBlob } from '@/utils/download-blob'

// The level requested here is a hint to the wallet only — the backend gates
// on the contract's OWN declared per-field requirement regardless of what is
// requested (ADR-20 SM-01). AES covers every contract that doesn't declare a
// stricter requirement; a contract requiring QES needs a wallet/QTSP capable
// of producing one, which this desktop-upload path does not orchestrate.
// The level this ceremony TARGETS, sent with the prepare request. What the
// signature actually achieved is determined server-side and reported in the
// compliance view; this constant must never be presented as that result.
const CREDENTIAL_TARGET = 'AES'

const route = useRoute()
const router = useRouter()
const did = computed(() => (Array.isArray(route.params.did) ? route.params.did[0] : route.params.did) ?? '')

const { isSigner, isManager } = useContractPermissions()
const localInstanceDid = ref('')

// Render the real contract content (clauses/terms) the same way the contract
// views do: preprocess the JSON-LD into the draft store and hand it to TemplatePreview.
const { preprocessContractData } = useContractDataPreprocess()
const dcsDraftStore = useDcsDraftStore()
const contractContentValuesStore = useContractContentValuesStore()
const hasContent = ref(false)

const ceremonyDialog = useTemplateRef<InstanceType<typeof SigningCeremonyDialog>>('ceremony-dialog')

const contract = ref<SignatureContract | null>(null)
const loading = ref(true)
const loadError = ref<string | null>(null)

const envelope = ref<SignatureEnvelope | undefined>()
const verifyResult = ref<SignatureVerifyResult | undefined>()
const validateResult = ref<SignatureValidateResult | undefined>()
const provenanceChain = ref<ProvenanceEntry[]>([])

// After prepare, the contract awaits the externally-signed upload; holds the
// ceremony's signatory DID and ceremony id for the submit call (submit MUST
// resolve the SAME ceremony prepare pinned bytes on, ADR-20) and the
// to-be-signed PDF so the signer can download it again without re-running
// the identity ceremony.
const pendingSignerDid = ref<string | null>(null)
const pendingCeremonyId = ref<string | null>(null)
const preparedDocument = ref<Blob | null>(null)

const busy = ref(false)

type StepId = 'retrieve' | 'verify' | 'apply' | 'submit' | 'validate'
const stepError = ref<Partial<Record<StepId, string>>>({})

const done = ref<Record<StepId, boolean>>({
  retrieve: false,
  verify: false,
  apply: false,
  submit: false,
  validate: false,
})

const STEPS: { id: StepId; title: string }[] = [
  { id: 'retrieve', title: 'Open' },
  { id: 'verify', title: 'Verify' },
  { id: 'apply', title: 'Download to sign' },
  { id: 'submit', title: 'Upload signed' },
  { id: 'validate', title: 'Validate' },
]

const currentStep = computed<StepId>(() => STEPS.find((s) => !done.value[s.id])?.id ?? 'validate')

function stepState(id: StepId): 'done' | 'active' | 'pending' {
  if (done.value[id]) return 'done'
  return id === currentStep.value ? 'active' : 'pending'
}

const signed = computed(() => envelope.value?.status === 'SIGNED')
const executed = computed(() => done.value.validate && signed.value)

// The signature field to sign is the participating party's slot (dcs:signatoryName
// == the party's DCS instance DID), seeded on the contract at negotiation. The
// ceremony binds to it and the wallet's Power of Attorney must authorize that party.
const signatureFieldName = computed<string>(() => {
  const cd = contract.value?.contract_data as Record<string, unknown> | undefined
  const fields = cd?.['dcs:signatureFields'] as Record<string, unknown>[] | undefined
  const names = (fields ?? [])
    .map((field) => field['dcs:signatoryName'])
    .filter((name): name is string => typeof name === 'string')
  // Sign OUR OWN slot, identified by this DCS instance's did:web — NOT the
  // logged-in user's issuer, which is the signatory's organization and never
  // matches a party slot. Taking the first declared field made the counterparty
  // start a ceremony for the originator's slot, and the ceremony refuses a PoA
  // authorizing anyone but the party being signed for, so the counterparty could
  // never sign a two-party contract. Fall back to the sole field of a
  // single-signature contract, which declares no per-party slots.
  const own = names.find((name) => name === localInstanceDid.value)
  return own ?? names[0] ?? ''
})

onMounted(async () => {
  try {
    // This instance's own did:web identifies which signature slot is ours.
    localInstanceDid.value = (await getLocalDIDFile().catch(() => ({ id: '' }))).id
    const detail = await signatureManagementService.retrieveById(did.value)
    contract.value = detail.contract
    envelope.value = detail.signature_envelope
    loadContractContent(detail.contract.contract_data)
    done.value.retrieve = true
    await loadProvenance()
  } catch (e: unknown) {
    loadError.value = `Failed to retrieve the contract: ${message(e)}`
  } finally {
    loading.value = false
  }
})

// The C2PA provenance chain grows a manifest per lifecycle step and per applied
// signature; an unsigned/never-exported contract simply has none yet.
async function loadProvenance() {
  provenanceChain.value = (await signatureManagementService.getProvenanceChain(did.value)).chain
}

// A dcs.lifecycle assertion labels each manifest; surface its human-facing bits.
function provenanceStatus(entry: ProvenanceEntry): string {
  return entry.lifecycle?.status ?? entry.lifecycle?.action ?? 'manifest'
}
function provenanceActor(entry: ProvenanceEntry): string {
  return entry.lifecycle?.actor ?? entry.lifecycle?.signatory ?? entry.lifecycle?.contract_id ?? ''
}
function provenanceTime(entry: ProvenanceEntry): string {
  const raw = entry.lifecycle?.effective_at ?? entry.lifecycle?.timestamp ?? ''
  if (!raw) return ''
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

function loadContractContent(contractData: unknown) {
  const cd = contractData == null ? null : preprocessContractData(contractData)
  if (!cd) {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    hasContent.value = false
    return
  }
  dcsDraftStore.reset({
    workflow: 'contract',
    documentIri: ((contractData as Record<string, unknown>)['@id'] as string | undefined) ?? null,
    blocks: cd.blocks,
    layout: cd.layout,
    contractFields: cd.contractFields,
    contractData: cd.contractData,
    policies: cd.policies,
  })
  contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  hasContent.value = true
}

function message(e: unknown): string {
  const data = (e as { response?: { data?: { message?: unknown } } }).response?.data
  const serverMessage = typeof data?.message === 'string' ? data.message : ''
  if (serverMessage && isErasedContentMessage(serverMessage)) return ERASED_CONTENT_MESSAGE
  return e instanceof Error ? e.message : String(e)
}

const documentFilename = computed(() => `${contract.value?.name ?? did.value}-to-sign.pdf`)

function downloadAgain() {
  if (preparedDocument.value) downloadBlob(preparedDocument.value, documentFilename.value)
}

// Step 2 — integrity of the contract content and its signature envelope.
async function verify() {
  busy.value = true
  delete stepError.value.verify
  try {
    verifyResult.value = await signatureManagementService.verifySignature(did.value)
    // The verdict, not the call completing. A deterministic re-render that does
    // not match the signed document means the human-readable PDF and the
    // machine-readable contract are different documents, which is the property
    // this step exists to establish — so it must not read as Verified, and must
    // not open the ceremony.
    done.value.verify = verifyResult.value?.match === true
  } catch (e: unknown) {
    stepError.value.verify = `Verification failed: ${message(e)}`
  } finally {
    busy.value = false
  }
}

// Step 3 — run the PID ceremony, then fetch the to-be-signed PDF (PoA + summary
// embedded, signature field placed) for the signatory to sign externally (ADR-12).
async function applySignature() {
  busy.value = true
  delete stepError.value.apply
  try {
    const outcome = await ceremonyDialog.value?.reveal({ contractDid: did.value, fieldName: signatureFieldName.value })
    if (!outcome || outcome.isCanceled || !outcome.data) return
    const prepared = await signatureManagementService.prepareSignature(
      did.value,
      outcome.data.signerDid,
      CREDENTIAL_TARGET,
      signatureFieldName.value,
      outcome.data.ceremonyId,
    )
    preparedDocument.value = prepared
    downloadBlob(prepared, documentFilename.value)
    pendingSignerDid.value = outcome.data.signerDid
    pendingCeremonyId.value = outcome.data.ceremonyId
    done.value.apply = true
  } catch (e: unknown) {
    stepError.value.apply = `Could not prepare the contract for signing: ${signingErrorMessage(e)}`
  } finally {
    busy.value = false
  }
}

// Step 4 — upload the externally-signed PDF; the DCS validates sole control and records it.
async function submitSigned(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || !pendingSignerDid.value || !pendingCeremonyId.value) return
  busy.value = true
  delete stepError.value.submit
  try {
    envelope.value = await signatureManagementService.submitSignature(
      did.value,
      pendingSignerDid.value,
      CREDENTIAL_TARGET,
      file,
      signatureFieldName.value,
      pendingCeremonyId.value,
    )
    pendingSignerDid.value = null
    pendingCeremonyId.value = null
    done.value.submit = true
    await loadProvenance()
  } catch (e: unknown) {
    stepError.value.submit = `The signed contract was rejected: ${signingErrorMessage(e)}`
  } finally {
    busy.value = false
    input.value = ''
  }
}

// Step 5 — validate the applied signature(s) against trust policies.
async function validate() {
  busy.value = true
  delete stepError.value.validate
  try {
    validateResult.value = await signatureManagementService.validateSignature(did.value)
    done.value.validate = true
  } catch (e: unknown) {
    stepError.value.validate = `Validation failed: ${message(e)}`
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex items-center justify-between gap-3 border-b border-base-content/10 bg-base-100 p-4">
      <div>
        <p class="text-xs font-black tracking-widest text-base-content/70 uppercase">{{ route.meta.name }}</p>
        <h2 class="truncate text-2xl font-bold">{{ contract?.name ?? 'Contract' }}</h2>
      </div>
      <button class="btn btn-outline btn-sm" @click="router.push({ name: ROUTES.SIGNING.LIST })">Back to list</button>
    </div>

    <div v-if="loading" class="p-6 text-base-content/60">Loading contract…</div>
    <div v-else-if="loadError" class="p-6">
      <div class="alert alert-error">{{ loadError }}</div>
    </div>

    <div v-else class="grid min-h-0 flex-1 grid-cols-1 gap-0 lg:grid-cols-2">
      <!-- LEFT: contract content -->
      <section class="flex min-h-0 flex-col border-base-content/10 lg:border-r">
        <div class="flex items-center gap-2 border-b border-base-content/10 px-4 py-2">
          <h3 class="text-sm font-semibold">Contract document</h3>
          <span class="badge badge-ghost badge-sm">{{ contract?.state }}</span>
          <span v-if="signed" class="badge badge-sm badge-success">{{ envelope?.status }}</span>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto bg-base-200 p-4">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body gap-4">
              <div>
                <h4 class="text-lg font-bold">{{ contract?.name ?? 'Untitled contract' }}</h4>
                <p class="font-mono text-xs break-all text-base-content/70">{{ did }}</p>
                <p v-if="contract?.description" class="mt-2 text-sm whitespace-pre-line">{{ contract.description }}</p>
              </div>

              <!-- The actual contract content: clauses, terms, and filled values -->
              <div v-if="hasContent" class="border-t border-base-300 pt-4">
                <TemplatePreview
                  :layout="dcsDraftStore.layout"
                  :blocks="dcsDraftStore.blocks"
                  :semantic-conditions="dcsDraftStore.semanticConditions"
                  :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                />
              </div>
              <p v-else class="text-sm text-base-content/70 italic">This contract has no renderable clause content.</p>

              <p class="text-xs text-base-content/70">
                Review the full clauses and machine-readable terms above before signing. The to-be-signed PDF with the
                embedded PoA and signing summary is produced at the Apply Signature step.
              </p>
            </div>
          </div>

          <!-- C2PA provenance: the tamper-evident manifest chain embedded in the PDF -->
          <div
            v-if="provenanceChain.length"
            data-testid="provenance-chain"
            class="card mt-4 border border-base-300 bg-base-100 shadow-sm"
          >
            <div class="card-body gap-3">
              <div class="flex items-center gap-2">
                <h4 class="text-sm font-bold">C2PA provenance chain</h4>
                <span class="badge badge-ghost badge-sm">{{ provenanceChain.length }} manifest(s)</span>
              </div>
              <p class="text-xs text-base-content/50">
                Each lifecycle step and applied signature seals a C2PA manifest into the document. This is the
                tamper-evident provenance embedded in the PDF, oldest first.
              </p>
              <ol class="relative ml-1.5 border-l border-base-300">
                <li v-for="entry in provenanceChain" :key="entry.label" class="mb-4 ml-4">
                  <span class="absolute left-[-6.5px] mt-1 h-3 w-3 rounded-full border border-base-100 bg-primary" />
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="badge badge-outline badge-sm">{{ provenanceStatus(entry) }}</span>
                    <span v-if="provenanceTime(entry)" class="text-xs text-base-content/50">
                      {{ provenanceTime(entry) }}
                    </span>
                  </div>
                  <div v-if="provenanceActor(entry)" class="mt-0.5 text-xs">{{ provenanceActor(entry) }}</div>
                  <div class="font-mono text-[10px] break-all text-base-content/40">{{ entry.label }}</div>
                </li>
              </ol>
            </div>
          </div>
        </div>
      </section>

      <!-- RIGHT: guided signing wizard -->
      <section class="flex min-h-0 flex-col overflow-y-auto p-4">
        <ul class="steps steps-vertical mb-4 w-full lg:steps-horizontal">
          <li
            v-for="step in STEPS"
            :key="step.id"
            class="step"
            :class="{
              'step-primary': stepState(step.id) === 'done',
              'step-info': stepState(step.id) === 'active',
            }"
          >
            {{ step.title }}
          </li>
        </ul>

        <div v-if="executed" class="mb-4 alert alert-success">
          Executed contract submitted. All required signatures are applied and validated.
        </div>

        <div class="flex flex-col gap-4">
          <!-- Step 1: Retrieve -->
          <div class="card border border-base-300 bg-base-100">
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">1 · Open contract</h4>
                <span class="badge badge-sm badge-success">Opened</span>
              </div>
              <p class="text-xs text-base-content/70">
                Contract content loaded on the left for review. Version {{ contract?.contract_version ?? 1 }}.
              </p>
            </div>
          </div>

          <!-- Step 2: Verify -->
          <div
            class="card border bg-base-100"
            :class="stepState('verify') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">2 · Verify integrity &amp; envelope</h4>
                <span v-if="done.verify" class="badge badge-sm badge-success">Verified</span>
                <span v-else-if="verifyResult" class="badge badge-sm badge-error">Not verified</span>
              </div>
              <p class="text-xs text-base-content/70">
                pdf-core deterministically re-renders the PDF from the contract's embedded machine-readable payload and
                compares it to the signed document: a match proves the human-readable PDF (and its rendered payload
                hash) agrees with the machine-readable contract, and the C2PA manifest chain is intact.
              </p>
              <div
                v-if="verifyResult"
                class="text-xs"
                :class="verifyResult.match ? 'text-success-readable' : 'text-error-readable'"
              >
                Human ↔ machine match: {{ verifyResult.match ? 'deterministic re-render matches ✓' : 'mismatch ✗' }} ({{
                  verifyResult.sig_count
                }}
                signature(s))
              </div>
              <ul v-if="verifyResult?.findings?.length" class="list-disc pl-5 text-xs text-warning">
                <li v-for="(f, i) in verifyResult.findings" :key="i">{{ f }}</li>
              </ul>
              <div v-if="stepError.verify" class="text-xs text-error">{{ stepError.verify }}</div>
              <div class="card-actions">
                <button class="btn btn-sm btn-primary" :disabled="busy" @click="verify">
                  <span v-if="busy && currentStep === 'verify'" class="loading loading-xs loading-spinner" />
                  {{ done.verify ? 'Re-verify' : 'Verify' }}
                </button>
              </div>
            </div>
          </div>

          <!-- Step 3: Verify identity & download the document to sign -->
          <div
            class="card border bg-base-100"
            :class="stepState('apply') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">3 · Verify your identity &amp; download the document to sign</h4>
                <span v-if="done.apply" class="badge badge-sm badge-success">Downloaded</span>
              </div>
              <p class="text-xs text-base-content/70">
                Present your PID in the wallet ceremony. The DCS then builds the to-be-signed PDF (with your Power of
                Attorney and the signing summary embedded, targeting {{ CREDENTIAL_TARGET }}) and downloads it to your
                device. The DCS holds no signing key — you sign the document yourself.
              </p>
              <div v-if="!isSigner" class="text-xs text-warning">
                Your role can review and validate this contract, but signing requires the Signer role.
              </div>
              <div v-if="stepError.apply" class="text-xs text-error">{{ stepError.apply }}</div>
              <div class="card-actions">
                <button
                  class="btn btn-sm btn-primary"
                  :disabled="busy || !isSigner || !done.verify || signed"
                  @click="applySignature"
                >
                  <span v-if="busy && currentStep === 'apply'" class="loading loading-xs loading-spinner" />
                  {{ done.apply ? 'Verify & download again' : 'Verify identity & download document to sign' }}
                </button>
              </div>
              <div v-if="done.apply && !signed" class="mt-1 flex items-center gap-2 text-xs text-info">
                <span>
                  Document downloaded as “{{ documentFilename }}”. Sign it in your wallet, then upload it below.
                </span>
                <button class="btn btn-ghost btn-xs" :disabled="!preparedDocument" @click="downloadAgain">
                  Download again
                </button>
              </div>
            </div>
          </div>

          <!-- Step 4: Upload the externally-signed document -->
          <div
            class="card border bg-base-100"
            :class="stepState('submit') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">4 · Upload the signed document</h4>
                <span v-if="signed" class="badge badge-sm badge-success">Uploaded</span>
              </div>
              <p class="text-xs text-base-content/70">
                Once you have signed the downloaded PDF, upload it here. The DCS checks that the signing certificate
                names the same person as your identity credential, and records the executed contract. That is evidence
                towards sole control, not a determination of it.
              </p>
              <p v-if="!done.apply" class="text-xs text-base-content/70 italic">
                Complete step 3 first to get the document to sign.
              </p>
              <p v-else-if="signed" class="text-xs text-success">Signed document accepted and recorded.</p>
              <div v-if="stepError.submit" class="text-xs text-error">{{ stepError.submit }}</div>
              <div class="card-actions">
                <label class="btn btn-sm btn-primary" :class="{ 'btn-disabled': busy || !pendingSignerDid }">
                  <span v-if="busy && currentStep === 'submit'" class="loading loading-xs loading-spinner" />
                  Upload signed document
                  <input
                    type="file"
                    accept="application/pdf"
                    class="hidden"
                    :disabled="busy || !pendingSignerDid"
                    @change="submitSigned"
                  />
                </label>
              </div>
            </div>
          </div>

          <!-- Step 5: Validate -->
          <div
            class="card border bg-base-100"
            :class="stepState('validate') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">5 · Validate applied signatures</h4>
                <span v-if="done.validate" class="badge badge-sm badge-success">Validated</span>
              </div>
              <p class="text-xs text-base-content/70">Validate the applied signature(s) against trust policies.</p>
              <ul v-if="validateResult?.findings?.length" class="list-disc pl-5 text-xs text-warning">
                <li v-for="(f, i) in validateResult.findings" :key="i">{{ f }}</li>
              </ul>
              <p v-else-if="done.validate" class="text-xs text-success">
                Signature validation passed with no findings.
              </p>
              <div v-if="stepError.validate" class="text-xs text-error">{{ stepError.validate }}</div>
              <div class="card-actions">
                <button class="btn btn-sm btn-primary" :disabled="busy || !signed" @click="validate">
                  <span v-if="busy && currentStep === 'validate'" class="loading loading-xs loading-spinner" />
                  {{ done.validate ? 'Re-validate' : 'Validate' }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <p v-if="isManager && !isSigner" class="mt-4 text-xs text-base-content/70">
          Manager view: retrieval, verification and validation are available; signing is performed by a Signer.
        </p>
      </section>
    </div>

    <SigningCeremonyDialog ref="ceremony-dialog" />
  </div>
</template>
