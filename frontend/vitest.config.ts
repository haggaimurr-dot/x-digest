import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// Unit tests for utils, composables and small components, without the Nuxt
// runtime: `~` maps to the app root like in Nuxt, and tests/unit/setup.ts
// provides the auto-imported globals (ref, computed, useLang …).
export default defineConfig({
  // Cast: vitest bundles its own vite types, which differ from the app's.
  // import.meta.client is replaced at build time by Nuxt; tests run as the
  // browser build (vite's `define` does not reach import.meta here).
  plugins: [vue() as any, {
    name: 'nuxt-import-meta-client',
    transform: (code: string) => code.includes('import.meta.client') ? code.replaceAll('import.meta.client', 'true') : undefined
  }],
  resolve: { alias: { '~': fileURLToPath(new URL('./', import.meta.url)) } },
  test: {
    environment: 'happy-dom',
    include: ['tests/unit/**/*.test.ts'],
    setupFiles: ['tests/unit/setup.ts'],
    coverage: {
      provider: 'v8',
      include: ['composables/useFormat.ts', 'utils/**/*.ts', 'components/TagInput.vue', 'components/FilterCombo.vue'],
      exclude: ['utils/i18n.ts', 'utils/x-post.mjs']
    }
  }
})
