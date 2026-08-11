import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const workspacePath = new URL('../src/features/short-drama-preroll-v2/ShortDramaPrerollWorkspace.tsx', import.meta.url)
const apiPath = new URL('../src/data/api.ts', import.meta.url)

test('short drama preroll V3 uses one selected frame reference and persistent workspace commands', async () => {
  const [workspace, api] = await Promise.all([
    readFile(workspacePath, 'utf8'),
    readFile(apiPath, 'utf8'),
  ])

  assert.doesNotMatch(workspace, /fixtureAnalysis|fixtureHooks|fixtureImages/)
  assert.match(workspace, /api\.uploadProjectAsset/)
  assert.match(workspace, /requireAuthoritativeVideo/)
  assert.match(workspace, /findAuthoritativeVideo/)
  assert.doesNotMatch(workspace, /sourceType: 'upload' as const/)
  assert.match(workspace, /sourceUnavailableMessage/)
  assert.match(workspace, /api\.analyzeShortDramaV2Source/)
  assert.match(workspace, /current = await api\.reconcileShortDramaV2Video[\s\S]*job\.status !== 'succeeded'/)
  assert.match(workspace, /api\.generateShortDramaV2Directions/)
  assert.match(workspace, /direction_batch\?\.selected_direction_id/)
  assert.match(workspace, /output_canvas_asset/)
  assert.match(workspace, /version: 4/)
  assert.match(workspace, /restoreState\(currentProject\.id, restored, source, session\)/)
  assert.doesNotMatch(workspace, /prepareShortDramaV2OpeningFrame|bindShortDramaV2TrustedMaterials|trustedMaterialsBound/)
  assert.match(workspace, /Prompt \+ 单张首帧参考/)
  assert.match(workspace, /重新生成 3 张/)
  assert.match(workspace, /directionSelectionGate\.isActive\(\)/)
  assert.match(workspace, /hook-selection-started/)
  assert.match(workspace, /cause\.status === 412/)
  assert.match(workspace, /getShortDramaPrerollV2Workspace/)
  assert.equal(workspace.match(/onClick=\{\(\) => void generateHooks\(\)\}/g)?.length, 1)

  assert.match(api, /short-drama-preroll-v2:\$\{action\}/)
  assert.match(api, /route_manual_short_drama_preroll_v2/)
  assert.match(api, /short-drama-v3-\$\{action\}-\$\{taskId\}/)
  assert.doesNotMatch(api, /short-drama-v2-\$\{action\}-\$\{taskId\}-\$\{Date\.now/)
})
