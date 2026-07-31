<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'

interface Stage {
  name: string
  description: string
  roles: string
  routeName: string
  linkLabel: string
}

const router = useRouter()
const authStore = useAuthStore()

const stages: Stage[] = [
  {
    name: 'Semantic Hub',
    description:
      'Operators register versioned SHACL shapes, JSON-LD contexts and ontologies. These drive the typed clauses and the validation applied to every template and contract.',
    roles: 'Template Manager',
    routeName: ROUTES.SEMANTIC_HUB.DASHBOARD,
    linkLabel: 'Open Semantic Hub',
  },
  {
    name: 'Templates',
    description:
      'Template Creators draft reusable templates — prose sections, clauses and placeholders bound to requirement fields, plus typed clauses from the hub catalog. Each template passes review, approval and catalogue registration before contracts can use it.',
    roles: 'Template Creator, Template Reviewer, Template Approver, Template Manager',
    routeName: ROUTES.TEMPLATES.LIST,
    linkLabel: 'Open Templates',
  },
  {
    name: 'Contracts',
    description:
      'Contract Creators instantiate registered templates, fill in the required values and submit. The contract then moves through negotiation with the counterparty, review and approval.',
    roles:
      'Contract Creator, Contract Negotiator, Contract Reviewer, Contract Approver, Contract Manager, Contract Observer',
    routeName: ROUTES.CONTRACTS.LIST,
    linkLabel: 'Open Contracts',
  },
  {
    name: 'Signing',
    description:
      'Contract Signers apply verifiable signatures to approved contracts in the Secure Contract Viewer. Once all required signatures are in place, the contract is legally executed.',
    roles: 'Contract Signer, Contract Manager, Contract Observer',
    routeName: ROUTES.SIGNING.LIST,
    linkLabel: 'Open Signing',
  },
  {
    name: 'Lifecycle & KPIs',
    description:
      'Signed contracts go into force and are deployed to a target system that measures KPIs against the machine-readable terms. Renewals, termination and expiry are managed from the contract list.',
    roles: 'Contract Manager, Contract Observer',
    routeName: ROUTES.CONTRACTS.LIST,
    linkLabel: 'Open Contracts',
  },
  {
    name: 'Audit & Archive',
    description:
      'Every lifecycle action is recorded in a tamper-evident, IPFS-anchored trail. Auditors run scoped audits with exportable reports; archived contracts stay verifiable end to end.',
    roles: 'Auditor, Archive Manager',
    routeName: ROUTES.AUDIT.LIST,
    linkLabel: 'Open Audit',
  },
]

const canAccess = (routeName: string): boolean => {
  const route = router.getRoutes().find((r) => r.name === routeName)
  if (!route?.meta.roles) return true
  return authStore.user?.roles?.some((role) => route.meta.roles?.includes(role)) ?? false
}

const visibleStages = computed(() => stages.filter((stage) => canAccess(stage.routeName)))
</script>

<template>
  <div class="mx-auto flex max-w-3xl flex-col gap-6 p-6">
    <div>
      <h1 class="text-2xl/7 font-bold sm:text-3xl sm:tracking-tight">Digital Contracting Service</h1>
      <p class="mt-2 text-base-content/70">
        Templates, negotiated contracts, verifiable signatures and auditable archives — end to end.
      </p>
      <p class="mt-1 text-sm text-base-content/70">You see the stages your roles can act on.</p>
    </div>

    <div class="flex flex-col gap-4">
      <div v-for="stage in visibleStages" :key="stage.name" class="card border border-base-300 bg-base-100 shadow-sm">
        <div class="card-body gap-2">
          <div class="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
            <div>
              <h2 class="card-title text-base">{{ stage.name }}</h2>
              <p class="mt-1 text-sm text-base-content/80">{{ stage.description }}</p>
              <p class="mt-2 text-xs text-base-content/70">Acts here: {{ stage.roles }}</p>
            </div>
            <RouterLink :to="{ name: stage.routeName }" class="btn shrink-0 btn-outline btn-sm">
              {{ stage.linkLabel }}
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
