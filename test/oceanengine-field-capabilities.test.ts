import test from 'node:test'
import assert from 'node:assert/strict'
import { projectEditPermission } from '../scripts/oceanengine-field-capabilities.ts'

test('project permission requires the exact account and active object; missing flags stay unknown', () => {
  const payload = (extra: Record<string, unknown> = {}) => ({ code: 0, data: [{ Id: '123', AdvertiserId: '456', IsDel: 0, ...extra }] })
  assert.equal(projectEditPermission(payload({ CanEdit: true }), '456', '123'), true)
  assert.equal(projectEditPermission(payload({ CanEdit: false }), '456', '123'), false)
  assert.equal(projectEditPermission(payload(), '456', '123'), undefined)
  assert.equal(projectEditPermission(payload({ CanEdit: 'true' }), '456', '123'), undefined)
  assert.throws(() => projectEditPermission(payload(), '999', '123'))
  assert.throws(() => projectEditPermission(payload({ IsDel: 1 }), '456', '123'))
  assert.throws(() => projectEditPermission({ code: 1, data: [] }, '456', '123'))
})
