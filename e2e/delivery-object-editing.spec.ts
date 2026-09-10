import { observeField } from '../scripts/oceanengine-field-capabilities.ts'
import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { DeliveryIntent, DeliveryPlanObjectPreview, DeliveryPlatformEntityMapping, PlatformConfiguration } from '../src/api/delivery'

const projectId = 'object-editor-test'
const planId = 'plan-stable'
const configurationFixture = JSON.parse(readFileSync('docs/delivery/fixtures/delivery-platform-configuration-v2-oceanengine-valid.json', 'utf8')) as PlatformConfiguration
const intent = JSON.parse(readFileSync('docs/delivery/fixtures/delivery-intent-v1-valid.json', 'utf8')) as DeliveryIntent

for (const kind of ['project', 'promotion'] as const) {
  test(`direct ${kind} editor preserves identity and exposes only supported mutations`, async ({ page }) => {
    page.on('pageerror', error => { throw error })
    const configuration = structuredClone(configurationFixture)
    const ocean = configuration.payload.ocean_engine!
    ocean.promotions[0].budget_and_bidding = { currency: 'CNY', daily_budget_minor: 30000, bidding_strategy: 'stable_cost', charging_mode: 'CPC' }
    const otherPromotion = structuredClone(ocean.promotions[0])
    otherPromotion.promotion_draft_id = 'other-unit-stable'
    otherPromotion.promotion_name = '另一单元'
    ocean.promotions.push(otherPromotion)
    let currentConfiguration = configuration
    let version = 1
    const mapping: DeliveryPlatformEntityMapping = {
      id: `mapping-${kind}`, account_reference_id: '123', plan_id: planId, configuration_id: configuration.configuration_id,
      business_execution_id: 'source-execution', browser_rpa_run_id: 'source-run', internal_object_kind: kind,
      internal_object_id: kind === 'project' ? ocean.project.project_draft_id : ocean.promotions[0].promotion_draft_id,
      platform_object_kind: kind, platform_object_id: kind === 'project' ? '99901' : '99902', platform_status: 'enabled',
      status: 'confirmed', version: 2, created_at: '2026-09-10T00:00:00Z', updated_at: '2026-09-10T00:00:00Z',
    }
    const preview = (): DeliveryPlanObjectPreview => ({ plan_id: planId, version, objects: [{ kind, internal_id: mapping.internal_object_id, name: kind === 'project' ? ocean.project.project_name : ocean.promotions[0].promotion_name, mapping_id: mapping.id, platform_id: mapping.platform_object_id, action: version === 1 ? 'unchanged' : 'update', changed_fields: version === 1 ? [] : ['budget_and_bidding'], fields: [{ key: kind === 'project' ? 'project_name' : 'promotion_name', state: 'unverified', reason: '编辑路径尚未校准。' }] }] })
    const plan = () => ({ id: planId, organization_id: 'org-test', project_id: projectId, status: 'draft', platform: 'ocean_engine', source: 'mock', scenario: 'test', current_version_number: version, current_version: { plan_id: planId, version_number: version, contract_version: 'delivery-plan/v2', platform: 'ocean_engine', source: 'mock', scenario: 'test', platform_configuration: currentConfiguration, intent }, versions: [], created_at: mapping.created_at, updated_at: mapping.updated_at })
    await page.route(`**/api/delivery/v1/projects/${projectId}/**`, async route => {
      const url = new URL(route.request().url())
      if (url.pathname.endsWith('/objects')) return route.fulfill({ json: preview() })
      if (url.pathname.endsWith('/controlled-change-sets')) {
        expect(route.request().postDataJSON()).toEqual({ expected_mapping_version: 2, action: 'update_promotion_budget', current_daily_budget_minor: 30000, target_daily_budget_minor: 40000 })
        return route.fulfill({ json: { id: 'budget-change-1', version: 1 } })
      }
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON()
        expect(body.expected_version).toBe(1)
        const next = body.platform_configuration as PlatformConfiguration
        expect(next.payload.ocean_engine!.project).toEqual(ocean.project)
        expect(next.payload.ocean_engine!.promotions.map(value => value.promotion_draft_id)).toEqual(ocean.promotions.map(value => value.promotion_draft_id))
        expect(next.payload.ocean_engine!.promotions.slice(1)).toEqual(ocean.promotions.slice(1))
        expect(next.payload.ocean_engine!.promotions[0].budget_and_bidding!.daily_budget_minor).toBe(40000)
        currentConfiguration = next
        version++
        return route.fulfill({ json: plan() })
      }
      if (url.pathname.endsWith(`/plans/${planId}`)) return route.fulfill({ json: plan() })
      if (url.pathname.endsWith('/plans')) return route.fulfill({ json: { items: [plan()] } })
      return route.fulfill({ status: 405, json: { error: { message: 'Unexpected request' } } })
    })
    await page.route('**/object-editor-test-page', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{margin:0;font:14px sans-serif;--line:#ddd;--surface:#fff;--muted:#555}*{box-sizing:border-box}button,input{font:inherit}fieldset{min-width:0}</style></head><body><div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh'; RefreshRuntime.injectIntoGlobalHook(window); window.$RefreshReg$=()=>{}; window.$RefreshSig$=()=>type=>type; window.__vite_plugin_react_preamble_installed__=true;
      const React = (await import('/node_modules/.vite/deps/react.js')).default; const {createRoot} = (await import('/node_modules/.vite/deps/react-dom_client.js')).default;
      const {PlatformEntityEditor} = await import('/src/features/delivery-platform-entities/PlatformEntityEditor.tsx');
      await import('/src/features/delivery-platform-entities/delivery-platform-entities.css');
      createRoot(document.getElementById('root')).render(React.createElement(PlatformEntityEditor,{projectId:${JSON.stringify(projectId)},mapping:${JSON.stringify(mapping)},onClose:()=>document.getElementById('root').remove()}));
      </script></body></html>` }))
    await page.goto('/object-editor-test-page')
    await expect(page.getByLabel('对象名称')).toHaveAttribute('readonly', '')
    await expect(page.getByLabel('目标日预算')).toBeVisible()
    if (kind === 'project') {
      await expect(page.getByLabel('目标日预算')).toHaveAttribute('readonly', '')
      await expect(page.getByRole('button', { name: '生成预算变更供审批' })).toHaveCount(0)
      await expect(page.getByText('另一单元', { exact: true })).toBeVisible()
    } else {
      await page.getByLabel('目标日预算').fill('400')
      await page.getByRole('button', { name: '保存预算目标草稿' }).click()
      await expect(page.getByRole('status')).toContainText('平台 ID 保持不变')
      await page.getByLabel('平台当前日预算').fill('300')
      await expect(page.getByRole('button', { name: '生成预算变更供审批' })).toBeDisabled()
      await page.getByRole('checkbox').check()
      await page.getByRole('button', { name: '生成预算变更供审批' }).click()
      await expect(page.getByRole('region', { name: '确认单元预算变更' })).toContainText('300 元 → 400 元')
      await expect(page.getByRole('button', { name: '确认此变更并准备执行' })).toBeEnabled()
    }
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(page.getByLabel('对象名称')).toHaveCount(0)
  })
}

test('field observations distinguish disabled groups, mixed controls and missing fields', async ({ page }) => {
  await page.setContent(`<section id="locked"><div class="oc-switch-card-item disabled">A</div><div class="oc-switch-card-item disabled">B</div></section><section id="mixed"><input disabled><input></section><input id="name"><section id="unknown">text</section><div id="radio"><div class="ovui-radio-item ovui-radio-item--disabled">A</div></div>`)
  for (const [selector, state] of [['#locked', 'readonly'], ['#mixed', 'conditional'], ['#name', 'editable'], ['#unknown', 'unknown'], ['#missing', 'unknown'], ['#radio', 'readonly']]) {
    expect((await observeField(page, { field: 'test', selector })).state).toBe(state)
  }
})
