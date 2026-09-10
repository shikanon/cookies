import { chromium, type Page } from '@playwright/test'
import { readFile } from 'node:fs/promises'
import { basename } from 'node:path'
import { resolveSessionPlaywrightEndpoint } from './browser-rpa-edge-session.ts'

type Request = { account_id: string; object_id: string; kind: 'project' | 'promotion' }
type Availability = { state: 'editable' | 'readonly' | 'unknown' | 'conditional'; source: string; reason: string }
type LocatorSpec = { field: string; selector?: string | null; kind?: string; name?: string }
type RecordValue = Record<string, unknown>
function record(value: unknown): RecordValue | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as RecordValue : undefined
}

export function projectEditPermission(payload: unknown, accountID: string, objectID: string): boolean | undefined {
  const body = record(payload)
  if (body?.code !== 0 || !Array.isArray(body.data)) throw new Error('Project response missing')
  const rows = body.data.map(record).filter(row => row?.Id === objectID && row.AdvertiserId === accountID)
  if (rows.length !== 1 || rows[0]?.IsDel !== 0) throw new Error('Project identity mismatch')
  return typeof rows[0].CanEdit === 'boolean' ? rows[0].CanEdit : undefined
}

const fieldKeys: Record<string, string> = {
  marketing_product_kind: 'marketing_product_reference.kind', marketing_product_ref: 'marketing_product_reference',
  delivery_carrier: 'carrier', deep_optimization: 'deep_optimization_mode', placement: 'placement_strategy',
  billing_method: 'budget_and_bidding.charging_mode', bid_strategy: 'budget_and_bidding.bidding_strategy',
  'budget.mode': 'budget_and_bidding.budget_mode', 'budget.type': 'budget_and_bidding.budget_mode',
  'budget.amount': 'budget_and_bidding.daily_budget_minor', 'bid.amount': 'budget_and_bidding.bid_minor',
  'schedule.type': 'schedule.mode', 'schedule.start': 'schedule.start_time', 'schedule.end': 'schedule.end_time',
  'schedule.dayparting': 'schedule.dayparting', 'search.bid_coefficient': 'search_boost.bid_coefficient',
  'tracking.display': 'monitoring_references.display', 'tracking.action': 'monitoring_references.action',
  'product.primary_name': 'product_name', 'product.additional_name': 'product_name.additional',
  'product.selling_points': 'product_selling_points', direct_link: 'direct_link_reference', backup_link: 'landing_page_reference.backup',
}

export async function observeField(page: Page, spec: LocatorSpec): Promise<Availability> {
  const unknown: Availability = { state: 'unknown', source: 'platform_page', reason: '此页面未找到唯一、可见且可判断的控件。' }
  const locator = spec.selector ? page.locator(spec.selector) : spec.kind === 'placeholder' && spec.name ? page.getByPlaceholder(spec.name, { exact: true }) : undefined
  if (!locator || await locator.count() !== 1 || !await locator.isVisible()) return unknown
  const state = await locator.evaluate(el => {
    if (el.matches('.disabled,:disabled,[readonly],[aria-disabled="true"],[aria-readonly="true"]') || /(?:^|\s)[\w-]*--disabled(?:\s|$)/.test(el.className) || el.closest('[aria-disabled="true"],fieldset[disabled]')) return 'readonly'
    if (el.matches('input,textarea,select,button,[role="combobox"]')) return 'editable'
    const controls = Array.from(el.querySelectorAll('input,textarea,select,button,[role="radio"],[role="checkbox"],.ovui-radio,.ovui-radio-item,.oc-switch-card-item'))
    if (!controls.length) return 'unknown'
    const disabled = controls.filter(node => node.matches('.disabled,:disabled,[readonly],[aria-disabled="true"],[aria-readonly="true"]') || /(?:^|\s)[\w-]*--disabled(?:\s|$)/.test(node.className) || !!node.closest('[class*="--disabled"],.oc-switch-card-item.disabled,[aria-disabled="true"]')).length
    return disabled === controls.length ? 'readonly' : disabled ? 'conditional' : 'editable'
  })
  return { state, source: 'platform_page', reason: state === 'editable' ? '当前编辑页控件可用；提交时仍需平台校验。' : state === 'readonly' ? '当前编辑页控件被禁用或设为只读。' : state === 'conditional' ? '此组控件部分可用，部分受限。' : unknown.reason }
}

export async function inspectFieldCapabilities(request: Request, sessionFile: string) {
  if (!/^\d+$/.test(request.account_id) || !/^\d+$/.test(request.object_id) || !['project', 'promotion'].includes(request.kind)) throw new Error('Invalid object identity')
  const browser = await chromium.connectOverCDP(await resolveSessionPlaywrightEndpoint(sessionFile), { timeout: 15_000 })
  let page: Page | undefined
  try {
    const context = browser.contexts()[0]
    if (!context) throw new Error('Edge context missing')
    page = await context.newPage()
    page.setDefaultTimeout(15_000)
    const base = 'https://ad.oceanengine.com'
    let objectCanEdit: boolean | undefined
    let projectID = request.object_id
    if (request.kind === 'project') {
      const url = new URL('/superior/api/v2/project', base)
      url.search = new URLSearchParams({ aadvid: request.account_id, project_ids: request.object_id, need_raw_campaign: 'true', need_keywords: 'true', need_product_regulation_opt_status: 'true', need_blue_flow_package_active: 'true', need_fill_history_blue_keywords_info: 'true' }).toString()
      const response = await context.request.get(url.toString(), { timeout: 15_000 })
      if (!response.ok()) throw new Error('Project request failed')
      objectCanEdit = projectEditPermission(await response.json(), request.account_id, request.object_id)
    } else {
      const url = new URL('/superior/api/ad/promotion/detail', base)
      url.search = new URLSearchParams({ aadvid: request.account_id, promotion_ids: request.object_id, need_invisible_material: 'false', need_material_group: 'true' }).toString()
      const response = await context.request.get(url.toString(), { timeout: 15_000 })
      const body = record(await response.json())
      const row = record(record(body?.data)?.[request.object_id])
      if (!response.ok() || body?.code !== 0 || row?.id !== request.object_id || row.advertiser_id !== request.account_id || row.is_del !== 0 || typeof row.project_id !== 'string' || !/^\d+$/.test(row.project_id)) throw new Error('Promotion identity mismatch')
      projectID = row.project_id
    }
    const url = new URL(request.kind === 'project' ? '/superior/create-project' : '/superior/ads', base)
    url.search = new URLSearchParams({ aadvid: request.account_id, project_id: projectID, is_update: '1', cascade_id: crypto.randomUUID(), uuid: crypto.randomUUID(), ...(request.kind === 'promotion' ? { promotion_id: request.object_id, temp_id: crypto.randomUUID() } : {}) }).toString()
    await page.goto(url.toString(), { waitUntil: 'domcontentloaded', timeout: 20_000 })
    await page.locator(request.kind === 'project' ? '[data-e2e="createproject_projectname"] textarea' : '[data-e2e="createad_adName"] textarea').waitFor({ state: 'visible' })
    const current = new URL(page.url())
    if (current.hostname !== 'ad.oceanengine.com' || current.searchParams.get('aadvid') !== request.account_id || current.searchParams.get('project_id') !== projectID || (request.kind === 'promotion' && current.searchParams.get('promotion_id') !== request.object_id)) throw new Error('Edit page identity mismatch')
    const fixture = JSON.parse(await readFile('docs/delivery/fixtures/oceanengine-existing-object-live-locators-v0.1.json', 'utf8')) as Record<string, { locked_locators: LocatorSpec[]; editable_locators: LocatorSpec[]; conditional_locators?: LocatorSpec[] }>
    const surface = fixture[`${request.kind}_edit`]
    const specs = [...surface.locked_locators, ...surface.editable_locators, ...surface.conditional_locators ?? []]
    const observations = []
    for (const spec of specs) observations.push({ key: fieldKeys[spec.field] ?? spec.field, platform: await observeField(page, spec) })
    return { ...request, object_can_edit: objectCanEdit, observed_at: new Date().toISOString(), observations }
  } finally {
    if (page) await page.close().catch(() => {})
    await browser.close()
  }
}

if (basename(process.argv[1] ?? '') === 'oceanengine-field-capabilities.ts') {
  try {
    const chunks = []
    for await (const chunk of process.stdin) chunks.push(chunk)
    const request = JSON.parse(Buffer.concat(chunks).toString('utf8')) as Request
    const sessionFile = process.argv[process.argv.indexOf('--session-file') + 1]
    if (!sessionFile || sessionFile === process.argv[0]) throw new Error('Session file missing')
    process.stdout.write(JSON.stringify(await inspectFieldCapabilities(request, sessionFile)))
  } catch {
    process.stderr.write('Field inspection failed; verify authenticated Edge session and object identity.\n')
    process.exitCode = 1
  }
}
