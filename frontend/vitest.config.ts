import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// Unit tests for utils, composables and small components, without the Nuxt
// runtime: `~` maps to the app root like in Nuxt, and tests/unit/setup.ts
// provides the auto-imported globals (ref, computed, useLang …).
export default defineConfig({
  // Cast: vitest bundles its own vite types, which differ from the app's.
  plugins: [vue() as any],
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
