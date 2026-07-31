#!/usr/bin/env node
// Static safety net: every literal button label the e2e specs click
// (getByRole('button', { name: 'Label' })) must exist somewhere in the
// frontend source, so a renamed/removed button fails CI here in <1s instead of
// surfacing as an opaque 30s "locator resolved to 0 elements" hang mid-run.
//
// Scope is deliberately conservative to avoid false failures: only
// single-quoted STRING labels are checked. Regex names (getByRole(..., { name:
// /.../ })) and template/variable labels (`${name}`) are test-generated or
// dynamic and skipped — the point is to catch typos and removed static
// buttons, not to model dynamic UI.
import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const srcDir = path.resolve(here, '../src')

function walk(dir, filter) {
  const out = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) out.push(...walk(full, filter))
    else if (filter(entry.name)) out.push(full)
  }
  return out
}

// The full frontend source, concatenated once, is where a rendered button label
// must appear literally (a template may interpolate it, e.g.
// `{{ busy ? 'Saving…' : 'Save' }}`, but the literal token is still present).
const srcHaystack = walk(srcDir, (n) => n.endsWith('.vue') || n.endsWith('.ts'))
  .map((f) => readFileSync(f, 'utf8'))
  .join('\n')

// getByRole('button', { name: 'Label' }) — capture the single-quoted literal.
const buttonRe = /getByRole\(\s*'button'\s*,\s*\{\s*name:\s*'([^']+)'/g

const specFiles = walk(here, (n) => n.endsWith('.spec.ts') || n.endsWith('helpers.ts'))
const missing = new Map() // label -> Set(spec basenames)

for (const file of specFiles) {
  const text = readFileSync(file, 'utf8')
  for (const m of text.matchAll(buttonRe)) {
    const label = m[1]
    if (!srcHaystack.includes(label)) {
      if (!missing.has(label)) missing.set(label, new Set())
      missing.get(label).add(path.basename(file))
    }
  }
}

if (missing.size > 0) {
  console.error('E2E button-label check FAILED — labels clicked in specs but not found in frontend/src:')
  for (const [label, files] of missing) {
    console.error(`  '${label}'  (in ${[...files].join(', ')})`)
  }
  console.error('\nEither the button was renamed/removed, or the spec has a typo. Fix one of them.')
  process.exit(1)
}

console.log('E2E button-label check passed — every literal button label resolves to a frontend button.')
