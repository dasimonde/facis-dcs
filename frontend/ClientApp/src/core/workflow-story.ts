import { ExtrinsicLifecycle } from '@/types/extrinsic-lifecycle'
import type { ContractState } from '@/types/contract-state'
import type { ContractTemplateState } from '@/types/contract-template-state'
import type { TemplateType } from '@/types/template-type'
import type { RouteLocationRaw } from 'vue-router'

export interface StoryStep {
  key: string
  label: string
}

export interface StoryActionHint {
  label: string
  routeName?: string
}

export interface WorkflowStory {
  steps: StoryStep[]
  currentKey: string
  headline: string
  narrative: string
  actionHints: StoryActionHint[]
}

export interface StoryAction {
  label: string
  to?: RouteLocationRaw
}

export function toBannerActions(hints: StoryActionHint[]): StoryAction[] {
  return hints.map((hint) =>
    hint.routeName ? { label: hint.label, to: { name: hint.routeName } } : { label: hint.label },
  )
}

const TEMPLATE_STEPS: StoryStep[] = [
  { key: 'DRAFT', label: 'Draft' },
  { key: 'SUBMITTED', label: 'Submitted' },
  { key: 'REVIEWED', label: 'Reviewed' },
  { key: 'APPROVED', label: 'Approved' },
  { key: 'IN_USE', label: 'Registered (in use)' },
]

const COMPONENT_NOTE =
  'This Component can be added to Contract Templates but cannot be used to create contracts directly.'

export function templateStory(
  state: ContractTemplateState | null | undefined,
  opts?: { isEditableView?: boolean; templateType?: TemplateType | null },
): WorkflowStory {
  const isComponent = opts?.templateType === 'COMPONENT'
  const story = (
    currentKey: string,
    headline: string,
    narrative: string,
    actionHints: StoryActionHint[] = [],
  ): WorkflowStory => ({
    steps: TEMPLATE_STEPS,
    currentKey,
    headline,
    narrative,
    actionHints,
  })

  switch (state) {
    case 'DRAFT':
      return story(
        'DRAFT',
        'This template is a draft',
        opts?.isEditableView
          ? "Complete the template's content, including sections, clauses, placeholders and typed clauses from the Semantic Hub, then submit it for review. (Template Creator)"
          : 'The Template Creator completes the content, including sections, clauses, placeholders and typed clauses from the Semantic Hub, and submits it for review.',
      )
    case 'SUBMITTED':
      return story(
        'SUBMITTED',
        'This template is in review',
        "A Template Reviewer now verifies the template against the hub's SHACL shapes and policy rules, then forwards it to approval or returns it to draft. Reviewers find it under Review Tasks.",
        [{ label: 'Open Review Tasks', routeName: 'tasks.reviews' }],
      )
    case 'REVIEWED':
      return story(
        'REVIEWED',
        'This template awaits approval',
        'A Template Approver decides: approve it for contract use or reject it back to draft. Approvers find it under Approval Tasks.',
        [{ label: 'Open Approval Tasks', routeName: 'tasks.approvals' }],
      )
    case 'APPROVED':
      return story(
        'APPROVED',
        'This template is approved',
        isComponent
          ? `Approved. ${COMPONENT_NOTE}`
          : 'Approved. A Template Manager can now register it in the Template Catalogue so Contract Creators can use it to create contracts.',
        [{ label: 'Open Template Catalogue', routeName: 'template.catalogues.list' }],
      )
    case 'REJECTED':
      return story(
        'REJECTED',
        'This template was rejected',
        opts?.isEditableView
          ? 'It was returned with findings. Address the comments and resubmit. (Template Creator)'
          : 'It was returned with findings. The Template Creator addresses the comments and resubmits it for review.',
      )
    case 'DEPRECATED':
      return story(
        'DEPRECATED',
        'This template is deprecated',
        'Deprecated. New contracts can no longer be created from it, but existing contracts are unaffected.',
      )
    case 'DELETED':
      return story(
        'DELETED',
        'This template was deleted',
        'Deleted. It is no longer part of the working repository. Its history remains in the tamper-evident audit trail.',
      )
    case 'REGISTERED':
      return story(
        'IN_USE',
        'This template is registered in the catalogue',
        isComponent
          ? `Registered in the catalogue. ${COMPONENT_NOTE}`
          : 'Registered in the catalogue. Contract Creators can now create contracts from it.',
        isComponent
          ? [{ label: 'Open Template Catalogue', routeName: 'template.catalogues.list' }]
          : [{ label: 'Create a contract', routeName: 'contracts.new' }],
      )
    case 'PUBLISHED':
      return story(
        'IN_USE',
        'This template is published in the catalogue',
        isComponent
          ? `Published in the federated catalogue and discoverable by other participants. ${COMPONENT_NOTE}`
          : 'Published in the federated catalogue and discoverable by other participants. Contract Creators can now create contracts from it.',
        isComponent
          ? [{ label: 'Open Template Catalogue', routeName: 'template.catalogues.list' }]
          : [{ label: 'Create a contract', routeName: 'contracts.new' }],
      )
    default:
      return story(
        '',
        'Template lifecycle',
        'A template moves from draft through review, approval and catalogue registration before contracts can be created from it.',
      )
  }
}

const CONTRACT_STEPS: StoryStep[] = [
  { key: 'DRAFT', label: 'Draft' },
  { key: 'SUBMITTED', label: 'Submitted' },
  { key: 'REVIEWED', label: 'Reviewed' },
  { key: 'APPROVED', label: 'Approved' },
  { key: 'SIGNED', label: 'Signed' },
  { key: 'ACTIVE', label: 'Active' },
]

export function contractStory(
  state: ContractState | null | undefined,
  opts?: { extrinsicLifecycle?: string | null },
): WorkflowStory {
  const story = (
    currentKey: string,
    headline: string,
    narrative: string,
    actionHints: StoryActionHint[] = [],
  ): WorkflowStory => ({
    steps: CONTRACT_STEPS,
    currentKey,
    headline,
    narrative,
    actionHints,
  })

  switch (state) {
    case 'DRAFT':
      return story(
        'DRAFT',
        'This contract is a draft',
        'Fill in the required values under Contract Content. Placeholders and typed clauses carry the machine-readable terms. Then submit for review. (Contract Creator)',
      )
    case 'OFFERED':
      return story(
        'OFFERED',
        'This contract is offered to the counterparty',
        'The counterparty now responds: accepting and signing seals the agreement, proposing changes moves it into negotiation, and they may also reject it. (Contract Negotiator)',
        [{ label: 'Open Negotiation Tasks', routeName: 'tasks.negotiations' }],
      )
    case 'NEGOTIATION':
      return story(
        'NEGOTIATION',
        'This contract is in negotiation',
        'The counterparty proposed changes. Compare versions and respond under Negotiation Tasks. Each accepted adjustment creates a new version of the same contract.',
        [{ label: 'Open Negotiation Tasks', routeName: 'tasks.negotiations' }],
      )
    case 'SUBMITTED':
      return story(
        'SUBMITTED',
        'This contract is in review',
        "A Contract Reviewer now validates the filled-in values against the template's semantic rules, then forwards it to approval or returns it. Reviewers find it under Review Tasks.",
        [{ label: 'Open Review Tasks', routeName: 'tasks.reviews' }],
      )
    case 'REVIEWED':
      return story(
        'REVIEWED',
        'This contract awaits approval',
        'A Contract Approver decides: approve it for signing or reject it. Approvers find it under Approval Tasks.',
        [{ label: 'Open Approval Tasks', routeName: 'tasks.approvals' }],
      )
    case 'APPROVED':
      return story(
        'APPROVED',
        'This contract is approved for signing',
        'Approved and locked. Contract Signers now sign it in the Secure Contract Viewer; once all required signatures are applied it becomes legally executed.',
        [{ label: 'Open Signing', routeName: 'signing.list' }],
      )
    case 'SIGNED':
      // SIGNED is reached on the FIRST signature, so it says nothing about the
      // counterparty. Whether the agreement is executed is the backend's
      // extrinsic lifecycle, which checks every declared signature field
      // (ADR-13, DCS-FR-SM-10). Without that signal, say only what is certain.
      if (opts?.extrinsicLifecycle === ExtrinsicLifecycle.executed) {
        return story(
          'SIGNED',
          'This contract is signed',
          'All signatures are applied, so the contract is executed. It is archived with its signature evidence and C2PA-provenanced PDF.',
        )
      }
      return story(
        'SIGNED',
        'This contract is signed',
        'Your signature is applied and archived with its signature evidence and C2PA-provenanced PDF. The contract becomes executed once every declared signatory has signed.',
      )
    case 'ACTIVE':
      return story(
        'ACTIVE',
        'This contract is active',
        "In force. KPIs from the contract's semantics are measured against the deployed target system; renewals and termination are managed from here.",
      )
    case 'REJECTED':
      return story(
        'REJECTED',
        'This contract was rejected',
        'It was returned with findings. Address the comments, adjust the values under Contract Content and resubmit. (Contract Creator)',
      )
    case 'WITHDRAWN':
      return story(
        'WITHDRAWN',
        'This offer was withdrawn',
        'The originator withdrew the offer before the counterparty accepted. It will not proceed; a new contract can be created from the same template.',
      )
    case 'REVOKED':
      return story(
        'REVOKED',
        'This contract was revoked',
        'Revoked after signing. It is no longer in force. The revocation is recorded in the audit trail and reflected in the C2PA status of the PDF.',
      )
    case 'TERMINATED':
      return story(
        'TERMINATED',
        'This contract is terminated',
        'Terminated before its natural expiry. It is no longer in force. Its full history remains archived and auditable.',
      )
    case 'EXPIRED':
      return story(
        'EXPIRED',
        'This contract has expired',
        'It reached the end of its agreed term. It remains archived with its complete, tamper-evident audit trail.',
      )
    default:
      return story(
        '',
        'Contract lifecycle',
        'A contract is created from an approved template, then moves through negotiation, review, approval and signing into force.',
      )
  }
}
