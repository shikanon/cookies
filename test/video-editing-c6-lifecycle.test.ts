import assert from 'node:assert/strict'
import test from 'node:test'

import { editingApi } from '../src/features/video-editing/api'

test('C6 editor submits one frozen export through check approve and delivery', async () => {
	const requests: Array<{ url: string; method: string; body?: unknown; key?: string | null }> = []
	const originalFetch = globalThis.fetch
	globalThis.fetch = async (input, init) => {
		const url = String(input)
		const body = init?.body ? JSON.parse(String(init.body)) : undefined
		const headers = new Headers(init?.headers)
		requests.push({ url, method: init?.method ?? 'GET', body, key: headers.get('Idempotency-Key') })
		if (url.endsWith(':deliver')) return Response.json({ id: 'package_1', creative_version_id: 'version_1', edit_task_id: 'edit_1', content_hash: 'sha256:value', created_at: '2026-08-11T00:00:00Z' })
		return Response.json({ id: 'version_1', edit_task_id: 'edit_1', status: url.endsWith(':approve') ? 'approved' : url.endsWith(':check') ? 'checked' : 'created', content_hash: 'sha256:value', video_snapshot: { final_video: { asset_id: 'output', version: 1 }, editing: {} } })
	}
	try {
		await editingApi.submitVersion('project_1', 'edit_1', 'render_1', 7)
		await editingApi.checkVersion('project_1', 'version_1')
		await editingApi.approveVersion('project_1', 'version_1')
		await editingApi.deliverVersion('project_1', 'version_1')
	} finally { globalThis.fetch = originalFetch }
	assert.deepEqual(requests.map(item => item.url.replace(/^.*\/api\/creative\/v1/, '')), [
		'/projects/project_1/edit-tasks/edit_1/versions:submit',
		'/projects/project_1/creative-versions/version_1:check',
		'/projects/project_1/creative-versions/version_1:approve',
		'/projects/project_1/creative-versions/version_1:deliver',
	])
	assert.deepEqual(requests[0]?.body, { render_job_id: 'render_1', expected_timeline_version: 7 })
	assert.equal(requests[0]?.key, 'editing-submit-edit_1-7-render_1')
})
