<script setup lang="ts">
import { computed, ref } from 'vue'
import { useTemplatePermissions } from '@template-repository/composables/useTemplatePermissions'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { contractTemplateService } from '@/services/contract-template-service'
import { reportActionError } from '@/utils/report-action-error'
import type {
  ContractTemplatePdfVerifyResponse,
  ContractTemplateVerifyResponse,
  TemplateProvenanceCredential,
} from '@/models/responses/template-response'

type PanelAction = 'verify' | 'provenance' | 'pdf' | 'export'

const props = withDefaults(
  defineProps<{
    did: string
    /** The actions this view offers; the user's roles limit them further. */
    actions?: PanelAction[]
  }>(),
  { actions: () => ['verify', 'provenance', 'pdf', 'export'] },
)

const { isReviewer, isApprover, isManager, isContractCreator, hasTemplateRole } = useTemplatePermissions()

// The scopes of the backend endpoints.
const allowed = computed<Record<PanelAction, boolean>>(() => ({
  verify: isReviewer.value || isApprover.value || isManager.value || isContractCreator.value,
  provenance: hasTemplateRole.value || isContractCreator.value,
  pdf: isManager.value || isReviewer.value,
  export: hasTemplateRole.value,
}))
const can = (action: PanelAction) => props.actions.includes(action) && allowed.value[action]
const visible = computed(() => props.actions.some(can))

const verifyResult = ref<ContractTemplateVerifyResponse | null>(null)
const verifying = ref(false)
const credentials = ref<TemplateProvenanceCredential[] | null>(null)
const loadingProvenance = ref(false)
const pdfResult = ref<ContractTemplatePdfVerifyResponse | null>(null)
const checkingPdf = ref(false)
const { download, exporting } = useDocumentExport()

async function run(busy: typeof verifying, context: string, action: () => Promise<void>) {
  busy.value = true
  try {
    await action()
  } catch (error) {
    reportActionError(error, context)
  } finally {
    busy.value = false
  }
}

// DCS-FR-TR-20, DCS-FR-TR-24, DCS-IR-TR-05: findings block the review, warnings do not.
const verify = () =>
  run(verifying, 'Verify template', async () => {
    verifyResult.value = await contractTemplateService.verify({ did: props.did })
  })

// DCS-FR-TR-09: one provenance credential for each registered version.
const loadProvenance = () =>
  run(loadingProvenance, 'Load provenance', async () => {
    credentials.value = await contractTemplateService.provenance(props.did)
  })

const verifyPdf = () =>
  run(checkingPdf, 'Verify PDF', async () => {
    pdfResult.value = await contractTemplateService.verifyPdf(props.did)
  })

const exportBundle = () => download(() => contractTemplateService.exportBundle(props.did), `template-${props.did}.zip`)

function credentialRows(entry: TemplateProvenanceCredential): [string, string][] {
  const subject = entry.credential.credentialSubject
  const rows: [string, string | undefined][] = [
    ['Issuer', entry.credential.issuer],
    ['Valid from', entry.credential.validFrom],
    ['Template hash', subject.template_hash],
    ['Created by', subject.created_by],
    ['Reviewed by', subject.reviewed_by?.join(', ')],
    ['Approved by', subject.approved_by?.join(', ')],
    ['Registered by', subject.registered_by],
    ['Registrar holder', subject.registrar_holder_did],
  ]
  return rows.filter((row): row is [string, string] => !!row[1])
}
</script>

<template>
  <section
    v-if="visible"
    class="rounded-box border border-base-content/10 bg-base-200/40 p-4 text-sm"
    data-testid="template-verification-panel"
  >
    <h3 class="mb-2 font-medium">Verification and provenance</h3>
    <div class="flex flex-wrap gap-2">
      <button v-if="can('verify')" type="button" class="btn btn-outline btn-sm" :disabled="verifying" @click="verify">
        Verify template
      </button>
      <button
        v-if="can('provenance')"
        type="button"
        class="btn btn-outline btn-sm"
        :disabled="loadingProvenance"
        @click="loadProvenance"
      >
        Load provenance
      </button>
      <button v-if="can('pdf')" type="button" class="btn btn-outline btn-sm" :disabled="checkingPdf" @click="verifyPdf">
        Verify PDF
      </button>
      <button
        v-if="can('export')"
        type="button"
        class="btn btn-outline btn-sm"
        :disabled="exporting"
        @click="exportBundle"
      >
        Export bundle
      </button>
    </div>

    <div v-if="verifyResult" class="mt-3 space-y-2" data-testid="template-verify-result">
      <p v-if="verifyResult.findings.length === 0" class="alert alert-success" role="status">
        Verification completed with no findings.
      </p>
      <template v-else>
        <h4 class="font-medium">Findings</h4>
        <ul class="list-disc pl-5" aria-label="Verification findings" data-testid="template-verify-findings">
          <li v-for="(finding, i) in verifyResult.findings" :key="i" class="break-words">{{ finding }}</li>
        </ul>
      </template>
      <template v-if="verifyResult.warnings?.length">
        <h4 class="font-medium">Warnings</h4>
        <ul class="list-disc pl-5" aria-label="Verification warnings" data-testid="template-verify-warnings">
          <li v-for="(warning, i) in verifyResult.warnings" :key="i" class="break-words">{{ warning }}</li>
        </ul>
      </template>
    </div>

    <p v-if="pdfResult" class="mt-3" role="status" data-testid="template-pdf-result">
      {{ pdfResult.match ? 'The PDF matches the template.' : 'The PDF does not match the template.' }}
    </p>

    <div v-if="credentials" class="mt-3" data-testid="template-provenance-credentials">
      <p v-if="credentials.length === 0" class="text-base-content/60">No provenance credentials recorded.</p>
      <article
        v-for="entry in credentials"
        :id="`provenance-${entry.vc_id}`"
        :key="entry.vc_id"
        class="mt-2 rounded-box border border-base-content/10 p-3"
        data-testid="template-provenance-credential"
      >
        <h4 class="font-medium">Version {{ entry.version }}</h4>
        <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
          <dt class="text-base-content/70">Credential ID</dt>
          <dd class="break-all">{{ entry.vc_id }}</dd>
          <template v-if="entry.previous_vc_id">
            <dt class="text-base-content/70">Previous credential</dt>
            <dd class="break-all">
              <a class="link" :href="`#provenance-${entry.previous_vc_id}`">{{ entry.previous_vc_id }}</a>
            </dd>
          </template>
          <template v-for="[label, value] in credentialRows(entry)" :key="label">
            <dt class="text-base-content/70">{{ label }}</dt>
            <dd class="break-all">{{ value }}</dd>
          </template>
        </dl>
      </article>
    </div>
  </section>
</template>
