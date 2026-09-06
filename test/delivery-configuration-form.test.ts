import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import test from 'node:test'

const component = readFileSync(resolve(import.meta.dirname, '../src/components/DeliveryConfigurationPage.tsx'), 'utf8')
const executionWorkspace = readFileSync(resolve(import.meta.dirname, '../src/features/browser-rpa-execution/BrowserRpaExecutionWorkspace.tsx'), 'utf8')
const styles = readFileSync(resolve(import.meta.dirname, '../src/styles.css'), 'utf8')

test('delivery line-list fields preserve the editing draft until blur', () => {
  assert.match(component, /function LineListTextarea/)
  assert.match(component, /setDraft\(nextDraft\)/)
  assert.match(component, /onValuesChange\(lineListValues\(nextDraft, limit, unique\)\)/)
  assert.match(component, /setDraft\(normalized\.join\('\\n'\)\)/)
  assert.match(component, /placeholder="每行一个行动号召"/)
  assert.match(component, /placeholder="每行一个卖点"/)
  assert.match(component, /placeholder="每行一条文案"/)
})

test('preferred-media checkboxes keep a fixed native control size', () => {
  assert.match(styles, /\.delivery-config-check-grid input\[type="checkbox"\]/)
  assert.match(styles, /flex:\s*0 0 16px/)
  assert.match(styles, /min-height:\s*16px/)
})

test('manual direct links do not require an OceanEngine object binding', () => {
  assert.match(component, /direct_link_mode === 'manual'/)
  assert.match(component, /placeholder="请输入 tbopen:\/\//)
  assert.match(component, /手动链接不需要绑定巨量平台 ID/)
  assert.match(component, /missingRequiredFields\.has\('direct_link'\)/)
})

test('enumerated Runner paths and dynamic bid limits are visible before execution', () => {
  assert.match(component, /当前项目路径不能生成 Runner 计划/)
  assert.match(component, /<option value="lead_generation">销售线索<\/option>/)
  assert.match(component, /<option value="application">应用<\/option>/)
  assert.match(component, /<option value="product_catalog">商品<\/option>/)
  assert.match(component, /<option value="content_marketing">内容营销<\/option>/)
  assert.match(component, /value="short_video_image_text"/)
  assert.match(component, /resolveOceanEngineBidConstraint/)
  assert.match(component, /项目出价必须在 \$\{formatOceanEngineMoneyRange\(projectBidConstraint\)\}之间/)
  assert.match(component, /由当前优化目标决定，不能单独修改/)
  assert.match(component, /销售线索项目必须设置日预算，且不能低于 300 元/)
  assert.match(component, /销售线索页面要求设置日预算/)
})

test('sales-lead optimization targets come from the exact account capability branch', () => {
  assert.match(component, /function OptimizationTargetCapabilityField/)
  assert.match(component, /当前账户和分支返回 \{snapshot\.options\.length\} 个优化目标/)
  assert.match(component, /oceanEngineLeadCaptureMode/)
  assert.match(component, /readProjectOptimizationTargetCapabilities/)
  assert.match(component, /capability_snapshot_id/)
  assert.match(component, /优化目标能力已变化/)
  assert.doesNotMatch(component, /当前分支只允许“点击量”或“展示量”/)
  assert.match(component, /if \(project\.marketing_purpose === 'lead_generation'\) project\.delivery_mode = 'ubmax'/)
  assert.match(component, /UBMax（平台固定）/)
  assert.match(component, /销售线索页面固定使用 delivery_mode=3。Runner 不操作此字段。/)
  assert.match(component, /\['stable_cost', 'cost_cap'\]\.includes\(ocean\.project\.budget_and_bidding\.bidding_strategy\)/)
})

test('multi-lead plans accept only Orange landing pages qualified for the selected optimization target', () => {
  assert.match(component, /multi_lead_external_actions/)
  assert.match(component, /multi_conversion_eligible/)
  assert.match(component, /当前账户没有支持/)
  assert.match(component, /支持当前优化目标/)
})

test('marketing product selection uses unique_product_id from Connector metadata', () => {
  assert.match(component, /function connectorMarketingProductIDs/)
  assert.match(component, /metadata\.unique_product_id/)
  assert.match(component, /id: uniqueProductID/)
  assert.match(component, /unique_product_id: uniqueProductID/)
  assert.match(component, /product_id: productID/)
})

test('product targeting is limited to the product-catalog branch and uses one exclusive mode', () => {
  assert.match(component, /marketing_purpose === 'product_catalog' \? <fieldset className="delivery-config-inline-fieldset"><legend>商品定向<\/legend>/)
  assert.match(component, /<ToggleField label="RTA 重定向"/)
  assert.match(component, /<option value="region_match">地域匹配<\/option><option value="delivery_conditions">商品投放条件<\/option>/)
  assert.doesNotMatch(component, /商品定向 · RTA 跳转/)
  assert.doesNotMatch(component, /商品定向 · 地域匹配/)
  assert.match(component, /marketingPurpose === 'product_catalog' \? \{\} : \{ product_targeting: undefined \}/)
})

test('real execution selects one immutable Web API or Playwright driver', () => {
  assert.match(component, /useState<DeliveryExecutionDriver>\('oceanengine-web-api\/session\/v1'\)/)
  assert.match(component, /value="oceanengine-web-api\/session\/v1"/)
  assert.match(component, /value="playwright-rpa\/edge\/v3"/)
  assert.match(component, /创建执行后，驱动选择不能更改/)
  assert.match(styles, /\.delivery-config-driver-options label\.selected/)
  assert.match(executionWorkspace, /effectiveExecutionDriver\(run\),\s*`browser-rpa-retry-/)
})
