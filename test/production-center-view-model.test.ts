import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { productionQueryForView, productionViews } from '../src/features/production-center/viewModel'

test('the six frozen views map to authoritative production queries', () => {
  assert.deepEqual(productionViews, ['图片生成', '视频生成', '音频生成', '渲染队列', '源素材', '失败任务'])
  assert.deepEqual(productionQueryForView('图片生成'), { media_kind: 'image', limit: 50 })
  assert.deepEqual(productionQueryForView('视频生成'), { media_kind: 'video', limit: 50 })
  assert.deepEqual(productionQueryForView('音频生成'), { media_kind: 'audio', limit: 50 })
  assert.deepEqual(productionQueryForView('渲染队列'), { media_kind: 'render', limit: 50 })
  assert.deepEqual(productionQueryForView('失败任务'), { status: ['failed', 'expired', 'partially_succeeded'], limit: 50 })
  assert.equal(productionQueryForView('源素材'), null)
})

test('production center is wired as a dedicated page instead of OperationsSurface', () => {
  const source = readFileSync(new URL('../src/components/Pages.tsx', import.meta.url), 'utf8')
  assert.match(source, /item\.id === 'production'.*ProductionCenterPage/s)
  assert.doesNotMatch(source, /item\.id === 'production'\s*\?\s*<OperationsSurface/)
})

test('production detail renders owner event error codes without deriving logs in the browser', () => {
  const source = readFileSync(new URL('../src/features/production-center/ProductionRunDrawer.tsx', import.meta.url), 'utf8')
  assert.match(source, /event\.error_code/)
  assert.match(source, /event\.safe_message/)
})
