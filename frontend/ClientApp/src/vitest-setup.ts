import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

/**
 * Unit tests run without a backend, but two vocabulary modules fetch the
 * Semantic Hub at import time (top-level await). The stub answers those routes
 * from the hub's own committed assets, so the ODRL profile the builders emit is
 * the real ones. The inventory includes the FACIS SLA ontology so catalog-
 * driven controls are tested against the same role vocabulary as production.
 */
const HUB_ASSETS = resolve(process.cwd(), '../../backend/internal/semantichub/assets')

const ontologies: Record<string, string> = {
  'dcs-odrl-profile': 'dcs-odrl-profile.ttl',
  'facis-sla-ontology': 'facis-sla-ontology.ttl',
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

globalThis.fetch = (input: RequestInfo | URL) => {
  const route = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url
  if (route === '/api/semantic/schema/list') {
    return Promise.resolve(
      json([{ name: 'facis-sla-ontology', kind: 'ontology', active_version: 1, latest_version: 1 }]),
    )
  }
  const ontology = route.startsWith('/api/semantic/ontology/')
    ? ontologies[decodeURIComponent(route.slice('/api/semantic/ontology/'.length))]
    : undefined
  if (ontology) {
    return Promise.resolve(json({ content: readFileSync(resolve(HUB_ASSETS, ontology), 'utf8') }))
  }
  return Promise.resolve(new Response(`no unit-test stub for ${route}`, { status: 404 }))
}
