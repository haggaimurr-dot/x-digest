// Nuxt 4.6.2 — requires Node >= 20.19 (verified here against Node v24.22.1).
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: false },
  css: ['~/assets/css/original.css', '~/assets/css/app.css'],
  app: {
    head: {
      title: 'x-digest · X 博主推文日报',
      htmlAttrs: { lang: 'zh-CN' },
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }]
    }
  },
  runtimeConfig: {
    // SSR-side API base only. Browser side always uses relative /api/v1/...
    // (nginx in production proxies /api/ to the Go API on 127.0.0.1:18090).
    serverApiBase: process.env.NUXT_SERVER_API_BASE || 'http://127.0.0.1:18090/api/v1',
    public: {
      // Mock/dev fallback, OFF by default. Enable with NUXT_PUBLIC_USE_MOCK=1.
      useMock: process.env.NUXT_PUBLIC_USE_MOCK === '1'
    }
  },
  nitro: {
    // Dev convenience only: when no nginx is in front, proxy /api to the Go API
    // (or to nothing when mock mode is on — mock mode never calls /api).
    devProxy: process.env.NUXT_PUBLIC_USE_MOCK === '1'
      ? undefined
      : { '/api/': { target: (process.env.NUXT_SERVER_API_BASE?.replace(/\/api\/v1$/, '') || 'http://127.0.0.1:18090') + '/api/', changeOrigin: true } }
  }
})
