import assert from 'node:assert/strict'
import test from 'node:test'
import viteConfig from '../vite.config.ts'

test('Go product API requests are proxied before the compatibility API', () => {
  assert.equal(typeof viteConfig, 'object')
  if (typeof viteConfig !== 'object' || viteConfig === null) {
    throw new Error('vite config must be an object')
  }

  const proxy = viteConfig.server?.proxy
  assert.ok(proxy && !Array.isArray(proxy), 'vite proxy configuration is required')
  const entries = Object.entries(proxy)
  const strategyIndex = entries.findIndex(([path]) => path === '/api/strategy/v1')
  const compatibilityIndex = entries.findIndex(([path]) => path === '/api')

  assert.equal(entries[0]?.[0], '/api/strategy/v1')
  for (const path of ['/api/connector/v1', '/api/creative/v1', '/api/delivery/v1', '/api/insights/v1', '/api/media/v1', '/api/platform/v1', '/api/strategy/v1']) {
    assert.equal(entries.find(([entryPath]) => entryPath === path)?.[1], process.env.VITE_PLATFORM_PROXY_TARGET ?? 'http://127.0.0.1:8080')
  }
  assert.ok(strategyIndex >= 0 && strategyIndex < compatibilityIndex, 'Strategy API must be matched before the compatibility API')
  assert.equal(entries[compatibilityIndex]?.[1], process.env.VITE_COMPAT_API_PROXY_TARGET ?? 'http://127.0.0.1:8787')
})

test('runtime-generated files are excluded from the Vite watcher', () => {
  assert.equal(typeof viteConfig, 'object')
  if (typeof viteConfig !== 'object' || viteConfig === null) {
    throw new Error('vite config must be an object')
  }

  const ignored = viteConfig.server?.watch?.ignored
  assert.ok(Array.isArray(ignored), 'vite watcher exclusions must be an array')
  assert.deepEqual(
    ignored,
    ['**/.codex-runtime/**', '**/.cache/**', '**/.data/**', '**/dist/**', '**/coverage/**'],
  )
})
