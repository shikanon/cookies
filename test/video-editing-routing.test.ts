import assert from 'node:assert/strict'
import test from 'node:test'

import { parseRoute, videoEditingPath } from '../src/lib/router.ts'

test('video editing task uses a stable project deep link and restores its task id', () => {
  const path = videoEditingPath('project 1', 'edit/task-9')
  assert.equal(path, '/projects/project%201/creative/video/editing/edit%2Ftask-9')

  const route = parseRoute(path)
  assert.equal(route.projectId, 'project 1')
  assert.equal(route.systemKey, 'creative')
  assert.equal(route.navId, 'video')
  assert.equal(route.view, '素材剪辑')
  assert.equal(route.contextId, 'edit/task-9')
})
test('legacy material editing query remains readable for redirect', () => {
  const route = parseRoute('/projects/project_1/creative/video?view=%E7%B4%A0%E6%9D%90%E5%89%AA%E8%BE%91&context=edit_1')
  assert.equal(route.contextId, 'edit_1')
  assert.equal(route.isLegacyVideoEditingRoute, true)
})
