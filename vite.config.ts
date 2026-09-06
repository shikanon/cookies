import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The compatibility service owns the legacy /api surface, while Strategy,
// Creative, Delivery, Insights, and /platform are served by the Go product API. Keep the more specific product
// prefixes before /api so Vite does not route them to the compatibility service.
const platformProxyTarget = process.env.VITE_PLATFORM_PROXY_TARGET ?? 'http://127.0.0.1:8080'
const compatibilityApiProxyTarget = process.env.VITE_COMPAT_API_PROXY_TARGET ?? 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [react()],
  server: {
    // Runtime media, databases, binaries, and generated reports are not source
    // inputs. On Windows these files may be locked while in use; excluding them
    // prevents an EBUSY watcher error from terminating the development server.
    watch: {
      ignored: ['**/.codex-runtime/**', '**/.cache/**', '**/.data/**', '**/dist/**', '**/coverage/**'],
    },
    proxy: {
      '/api/strategy/v1': platformProxyTarget,
      '/api/creative/v1': platformProxyTarget,
      '/api/delivery/v1': platformProxyTarget,
      '/api/connector/v1': platformProxyTarget,
      '/api/insights/v1': platformProxyTarget,
      '/api/media/v1': platformProxyTarget,
      '/api/platform/v1': platformProxyTarget,
      '/api': compatibilityApiProxyTarget,
      '/platform': platformProxyTarget,
    },
  },
})
