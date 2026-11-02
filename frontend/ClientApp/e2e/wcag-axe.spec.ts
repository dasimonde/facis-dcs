import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import AxeBuilder from '@axe-core/playwright'
import { expect, type Route, test } from '@playwright/test'

/**
 * WCAG 2.1 A/AA audit of every view (DCS-NFR-SQ-07) with axe-core. The API is
 * mocked: the Semantic Hub vocabularies the app loads at start come from the
 * hub's committed assets (as in vitest-setup.ts), every other route answers
 * 404. The audit covers each view's layout, navigation, forms and empty or
 * error states, and needs no running DCS instance.
 */

const HUB_ASSETS = resolve(process.cwd(), '../../backend/internal/semantichub/assets')
const ontologies: Record<string, string> = {
  'dcs-odrl-profile': 'dcs-odrl-profile.ttl',
  'facis-sla-ontology': 'facis-sla-ontology.ttl',
}

const did = encodeURIComponent('did:web:example.test:contract:1')

// The login views are audited signed out: signed in, '/' redirects.
const signedOutViews = ['/', '/pid-verify']
const views = [
  ...signedOutViews,
  '/frontpage',
  '/templates',
  '/templates/new',
  `/templates/edit/${did}`,
  `/templates/view/${did}`,
  `/templates/review/${did}`,
  `/templates/approve/${did}`,
  '/tasks/reviews',
  '/tasks/approvals',
  '/tasks/negotiations',
  '/catalogues/templates',
  `/catalogues/templates/view/${did}`,
  '/audit',
  `/archive/detail?did=${did}`,
  '/archive/dashboard',
  '/contracts',
  '/contracts/new',
  `/contracts/edit/${did}`,
  `/contracts/view/${did}`,
  `/contracts/negotiate/${did}`,
  `/contracts/review/${did}`,
  `/contracts/approve/${did}`,
  '/signing',
  `/signing/${did}`,
  '/compliance',
  '/admin/targets',
  '/admin/system-users',
  '/admin/hsm-keys',
  '/admin/lockouts',
  '/non-compliance',
  '/semantic-hub',
]

const allRoles = [
  'Template Creator',
  'Template Reviewer',
  'Template Approver',
  'Template Manager',
  'Contract Creator',
  'Contract Reviewer',
  'Contract Approver',
  'Contract Manager',
  'Contract Negotiator',
  'Contract Signer',
  'Contract Observer',
  'Archive Manager',
  'Auditor',
  'Sys. Administrator',
  'Compliance Officer',
  'Integration Manager',
]

function jwt(): string {
  const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url')
  const payload = {
    sub: 'did:web:user.example.test',
    exp: Math.floor(Date.now() / 1000) + 3600,
    roles: allRoles,
    ext: { iss: 'did:web:example.test', roles: allRoles },
  }
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode(payload)}.unsigned`
}

test.describe('WCAG 2.1 AA (axe)', () => {
  const token = jwt()

  test.beforeEach(async ({ page }) => {
    const json = (route: Route, body: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    await page.route(
      (url) => url.pathname.startsWith('/api/'),
      (route) => {
        const path = new URL(route.request().url()).pathname
        if (path === '/api/auth/refresh') return json(route, { token_type: 'Bearer', access_token: token })
        if (path === '/api/semantic/schema/list') {
          return json(route, [{ name: 'facis-sla-ontology', kind: 'ontology', active_version: 1, latest_version: 1 }])
        }
        const ontology = ontologies[decodeURIComponent(path.slice('/api/semantic/ontology/'.length))]
        if (path.startsWith('/api/semantic/ontology/') && ontology) {
          return json(route, { content: readFileSync(resolve(HUB_ASSETS, ontology), 'utf8') })
        }
        return json(route, { name: 'not_found', message: 'not found' }, 404)
      },
    )
  })

  for (const view of views) {
    test(`${view} has no WCAG 2.1 A/AA violations`, async ({ page }) => {
      if (!signedOutViews.includes(view)) {
        await page.addInitScript(
          ([accessToken]) => {
            localStorage.setItem('token_type', 'Bearer')
            localStorage.setItem('access_token', accessToken)
          },
          [token],
        )
      }
      await page.goto(`/ui${view}`)
      await page.waitForLoadState('networkidle')
      // Contrast is judged on the settled page, not on a toast fading in.
      await page.waitForFunction(() =>
        document
          .getAnimations()
          .every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity),
      )
      const { violations } = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .analyze()
      expect(
        violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`),
      ).toEqual([])
    })
  }
})
