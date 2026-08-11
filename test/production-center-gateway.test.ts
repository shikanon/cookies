import assert from 'node:assert/strict'
import test from 'node:test'

import { HttpProductionCenterGateway } from '../src/features/production-center/httpGateway'

test('production center gateway maps the frozen run filters and aborts stale list requests', async () => {
  const calls: Array<{ url: string; signal?: AbortSignal }> = []
  const pending: Array<(response: Response) => void> = []
  const fetcher: typeof fetch = (input, init) => new Promise<Response>(resolve => {
    calls.push({ url: String(input), signal: init?.signal ?? undefined })
    pending.push(resolve)
  })
  const gateway = new HttpProductionCenterGateway(fetcher, 'http://cookies.test')

  const first = gateway.listRuns('project one', { media_kind: 'video', status: ['running', 'failed'], q: 'job_8', limit: 25 })
  const second = gateway.listRuns('project two', { media_kind: 'render', cursor: 'cursor-v1', limit: 25 })

  assert.equal(calls[0].signal?.aborted, true, 'the previous Project request remained active')
  assert.match(calls[0].url, /\/api\/creative\/v1\/projects\/project%20one\/production-runs\?/)
  assert.match(calls[0].url, /media_kind=video/)
  assert.match(calls[0].url, /status=running%2Cfailed/)
  assert.match(calls[0].url, /q=job_8/)
  assert.match(calls[1].url, /projects\/project%20two\/production-runs/)
  assert.match(calls[1].url, /cursor=cursor-v1/)

  pending[0](new Response('', { status: 499 }))
  pending[1](Response.json({ contract_version: 'creative-production-run-page/v1', project_id: 'project two', items: [], next_cursor: null, source_health: [] }))
  await assert.rejects(first)
  const page = await second
  assert.equal(page.project_id, 'project two')
})

test('production center gateway uses the production-lineage asset endpoint rather than the asset library', async () => {
  let requestedURL = ''
  const fetcher: typeof fetch = async input => {
    requestedURL = String(input)
    return Response.json({ contract_version: 'creative-production-asset-page/v1', project_id: 'project_1', items: [], next_cursor: null, source_health: [] })
  }
  const gateway = new HttpProductionCenterGateway(fetcher, 'http://cookies.test')

  await gateway.listAssets('project_1', { role: 'input', media_kind: 'video', run_source: 'editing_render', limit: 50 })

  assert.match(requestedURL, /\/api\/creative\/v1\/projects\/project_1\/production-assets\?/)
  assert.doesNotMatch(requestedURL, /\/platform\/v1\/projects\/project_1\/assets/)
})

test('the default browser transport does not retain an illegal fetch receiver', async () => {
  const original = globalThis.fetch
  let receiver: unknown
  globalThis.fetch = function (this: unknown) {
    receiver = this
    return Promise.resolve(new Response(JSON.stringify({ contract_version: 'creative-production-run-page/v1', project_id: 'project_1', items: [], next_cursor: null, source_health: [] })))
  } as typeof fetch
  try {
    const gateway = new HttpProductionCenterGateway(undefined, 'http://cookies.test')
    await gateway.listRuns('project_1', {})
    assert.equal(receiver, undefined)
  } finally {
    globalThis.fetch = original
  }
})

test('production retry posts the frozen route with a caller-owned idempotency key', async () => {
  let requestedURL = ''
  let requestedInit: RequestInit | undefined
  const fetcher: typeof fetch = async (input, init) => {
    requestedURL = String(input)
    requestedInit = init
    return Response.json({
      contract_version: 'creative-production-retry/v1', status: 'accepted',
      previous_run: { source: 'editing_render', id: 'render-old' },
      new_run: { source: 'editing_render', id: 'render-new' },
      source_task: { system: 'creative', object_type: 'edit_task', object_id: 'edit-1', display_name: null },
    }, { status: 202 })
  }
  const gateway = new HttpProductionCenterGateway(fetcher, 'http://cookies.test')

  const result = await gateway.retryRun('project_1', { source: 'editing_render', id: 'render-old' }, 'production_retry_1')

  assert.match(requestedURL, /production-runs\/editing_render\/render-old:retry$/)
  assert.equal(requestedInit?.method, 'POST')
  assert.equal((requestedInit?.headers as Record<string, string>)['Idempotency-Key'], 'production_retry_1')
  assert.equal(result.new_run.id, 'render-new')
})

test('production retry reads the direct frozen problem envelope', async () => {
  const fetcher: typeof fetch = async () => Response.json({
    contract_version: 'creative-production-problem/v1',
    code: 'PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW',
    message: 'Retry this run from its source workflow.',
    retryable: false,
    source_task: { system: 'creative.ai-native-ad', object_type: 'production_unit', object_id: 'unit-1', display_name: null },
  }, { status: 409 })
  const gateway = new HttpProductionCenterGateway(fetcher, 'http://cookies.test')

  await assert.rejects(
    gateway.retryRun('project_1', { source: 'provider', id: 'job-old' }, 'production_retry_2'),
    (error: unknown) => error instanceof Error && 'code' in error && error.code === 'PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW',
  )
})
