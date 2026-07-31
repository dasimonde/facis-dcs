import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'url'
import { defineConfig } from 'vitest/config'

// Unit tests for the pure document/policy builders. The e2e suite
// (playwright.config.ts) is untouched and stays the browser-level check.
export default defineConfig({
  plugins: [vue({ template: { compilerOptions: { isCustomElement: (tag) => tag === 'shacl-form' } } })],
  resolve: {
    alias: {
      '@template-repository': fileURLToPath(new URL('./src/modules/template-repository/', import.meta.url)),
      '@contract-workflow-engine': fileURLToPath(new URL('./src/modules/contract-workflow-engine/', import.meta.url)),
      '@template-catalogue': fileURLToPath(new URL('./src/modules/template-catalogue/', import.meta.url)),
      '@semantic-hub': fileURLToPath(new URL('./src/modules/semantic-hub/', import.meta.url)),
      '@core': fileURLToPath(new URL('./src/core/', import.meta.url)),
      '@': fileURLToPath(new URL('./src/', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['./src/vitest-setup.ts'],
  },
})
