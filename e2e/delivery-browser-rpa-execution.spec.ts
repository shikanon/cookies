import { expect, test } from '@playwright/test'
import { createServer } from 'node:http'
import { PlaywrightPageOperations } from '../scripts/browser-rpa-runner-v3'
import type { OceanEngineFormPlan } from '../scripts/oceanengine-form-plan-compiler'

const hash = 'a'.repeat(64)
const projectId = 'project_local'
const runId = 'browser_rpa_run_fake_1'

test('Runner selects a cached native video twice and rejects a mismatched video ID', async ({ page }) => {
  let returnedID = '7681605279024303402'
  let queries = 0
  const server = createServer((request, response) => {
    if (request.url?.startsWith('/superior/api/v2/creative/material/video/list/')) {
      let body = ''
      request.on('data', chunk => { body += chunk })
      request.on('end', () => {
        const query = JSON.parse(body)
        expect(query.item_url).toBe('https://www.douyin.com/video/7681605279024303402')
        queries += 1
        response.setHeader('content-type', 'application/json')
        response.end(JSON.stringify({ code: 0, data: { items: [{ item_id: returnedID, title: '校准视频' }] } }))
      })
      return
    }
    response.setHeader('content-type', 'text/html; charset=utf-8')
    response.end(`<div id="selected"></div><button data-e2e="createad_materialSelectedAdd_video">添加视频</button>
      <div class="ovui-drawer__wrap" data-e2e="createad_videoLib" style="display:none">
        <input placeholder="搜索视频链接"><div data-auto-id="create-material-card"><span>校准视频</span><input type="checkbox"></div>
        <div data-e2e="createad_videoLib"><button id="confirm">确定</button></div>
      </div><button id="save">保存并关闭</button>
      <script>
        const drawer=document.querySelector('.ovui-drawer__wrap');
        document.querySelector('[data-e2e="createad_materialSelectedAdd_video"]').onclick=()=>drawer.style.display='block';
        document.querySelector('#confirm').onclick=()=>{
          if(!drawer.querySelector('[type="checkbox"]').checked) return;
          document.querySelector('#selected').innerHTML='<div data-e2e="createad_materialSelectedVideo__createMaterialSelected">校准视频<button class="oc-create-material-card-close">移除</button></div>';
          document.querySelector('.oc-create-material-card-close').onclick=e=>e.target.parentElement.remove();
          drawer.style.display='none'; drawer.querySelector('[type="checkbox"]').checked=false;
        };
        window.submitCount=0; document.querySelector('#save').onclick=()=>window.submitCount++;
      </script>`)
  })
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
  try {
    const address = server.address()
    if (!address || typeof address === 'string') throw new Error('test server address is missing')
    await page.goto(`http://127.0.0.1:${address.port}/superior/ads?aadvid=123`)
    const operations = new PlaywrightPageOperations(page)
    const step: OceanEngineFormPlan['steps'][number] = {
      id: 'native-video', kind: 'field_action', field_key: 'promotion.base_materials', operation: 'open_reference_picker',
      value: { selection_kind: 'async_row', material_type: 'douyin_video', object_id: returnedID }, remote_write: false, blocked: false,
    }
    await operations.applyField(step)
    await operations.applyField(step)
    expect(queries).toBe(2)
    expect(await operations.readField(step)).toMatchObject({ object_id: returnedID, selected_count: 1 })
    await expect(page.locator('[data-e2e="createad_materialSelectedVideo__createMaterialSelected"]')).toHaveCount(1)
    returnedID = '999'
    await expect(operations.applyField(step)).rejects.toThrow('exactly the requested Douyin item')
    expect(await page.evaluate(() => (window as unknown as { submitCount: number }).submitCount)).toBe(0)
  } finally {
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  }
})

test('Runner searches the category leaf and can clear native search terms', async ({ page }) => {
  await page.setContent(`<div data-e2e="createad_yuntuCategory"><span>所属类别</span><input placeholder="请选择"></div>
    <div data-e2e="createad_yuntuCategory_popover_content" style="display:none"><input placeholder="请输入内容"><div id="results"></div></div>
    <div data-e2e="search_tag__createRecommendTag"><span class="oc-tag-text">旧词</span><button class="ovui-tag__close">删除</button><input></div>
    <script>
      const picker=document.querySelector('[data-e2e="createad_yuntuCategory_popover_content"]');
      document.querySelector('[placeholder="请选择"]').onclick=()=>picker.style.display='block';
      picker.querySelector('input').oninput=e=>{
        document.querySelector('#results').innerHTML=e.target.value==='综合类2C电商'?'<div class="ovui-cascader-search-option">零售/综合类2C电商</div>':'';
        const result=picker.querySelector('.ovui-cascader-search-option');
        if(result) result.onclick=()=>{document.querySelector('[placeholder="请选择"]').value=result.textContent;picker.style.display='none'};
      };
      document.querySelector('.ovui-tag__close').onclick=e=>{document.querySelector('.oc-tag-text').remove();e.target.remove()};
    </script>`)
  const operations = new PlaywrightPageOperations(page)
  await operations.applyField({ id: 'category', kind: 'field_action', field_key: 'promotion.category', operation: 'choose_exact_visible_option', scope: '所属类别', target: '请选择', value: '零售/综合类2C电商', remote_write: false, blocked: false })
  await expect(page.getByPlaceholder('请选择', { exact: true })).toHaveValue('零售/综合类2C电商')
  const searchStep: OceanEngineFormPlan['steps'][number] = { id: 'search', kind: 'field_action', field_key: 'promotion.search_terms', operation: 'configure_object', value: [], remote_write: false, blocked: false }
  await operations.applyField(searchStep)
  expect(await operations.readField(searchStep)).toEqual([])
})

test('Runner application picker selects the exact app ID and detects changed readback', async ({ page }) => {
  await page.setContent(`
    <div data-e2e="createproject_appselect_input">
      <div data-e2e="createproject_appselect_input__ocInput"><input placeholder="请输入应用下载链接或选择已有应用"></div>
      <div data-auto-id="app-select-success"></div>
    </div>
    <div data-e2e="createproject_appselect"><button>选择</button></div>
    <div class="ovui-modal__wrap" style="display:none">应用管理
      <input placeholder="输入应用名称或ID后回车搜索"><table><tbody></tbody></table>
    </div>
    <button id="submit">保存并关闭</button>
    <script>
      const modal = document.querySelector('.ovui-modal__wrap');
      const input = document.querySelector('[data-e2e="createproject_appselect_input__ocInput"] input');
      const success = document.querySelector('[data-auto-id="app-select-success"]');
      document.querySelector('[data-e2e="createproject_appselect"] button').onclick = () => modal.style.display='block';
      modal.querySelector('input').onkeydown = event => {
        if(event.key !== 'Enter') return;
        setTimeout(() => {
          modal.querySelector('tbody').innerHTML='<tr class="ovui-tr"><td>191511</td><td>测试应用</td><td><button>使用该应用包</button></td></tr><tr class="ovui-tr"><td>1915110</td><td>其他应用</td></tr>';
          modal.querySelector('button').onclick=()=>{
            modal.style.display='none';
            setTimeout(()=>{ input.value='https://example.test/download/package-hash'; success.textContent='测试应用(com.example.app)'; },150);
          };
        },150);
      };
      input.onblur=()=>{if(input.value.startsWith('https://'))success.textContent='测试应用(com.example.app)';};
      document.querySelector('#submit').onclick=()=>{throw new Error('unexpected platform write');};
    </script>
  `)
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'application', kind: 'field_action', page_kind: 'project_create', field_key: 'project.application_reference',
    operation: 'open_reference_picker', value: { selection_kind: 'async_row', object_id: '191511', label: '测试应用', basic_package_id: 'package-hash', package_name: 'com.example.app' },
    remote_write: false, blocked: false,
  }
  await operations.applyField(step)
  expect(await operations.readField(step)).toMatchObject({ object_id: '191511', download_url: 'https://example.test/download/package-hash' })
  await operations.applyField(step)
  expect(await operations.readField(step)).toMatchObject({ object_id: '191511' })
  await page.getByPlaceholder('请输入应用下载链接或选择已有应用', { exact: true }).fill('https://example.test/other')
  await expect(operations.readField(step)).rejects.toThrow('selected application changed')
  const link = { ...step, value: { selection_kind: 'async_row', object_id: 'https://example.test/download/package-hash' } }
  await operations.applyField(link)
  expect(await operations.readField(link)).toMatchObject({ download_url: 'https://example.test/download/package-hash' })
})

test('Runner application scenario confirms the draft reset and reads the checked choice', async ({ page }) => {
  await page.setContent(`
    <div data-e2e="createproject_apppromotiontype">
      <div class="ovui-radio-item ovui-radio-item--checked">应用下载</div><div class="ovui-radio-item">应用调起</div>
    </div>
    <div class="ovui-modal__wrap" style="display:none">切换营销目的将会清空部分已填写的营销产品与目标，是否继续切换？<button>确定</button></div>
    <script>
      const choices=document.querySelectorAll('.ovui-radio-item'); const modal=document.querySelector('.ovui-modal__wrap');
      choices[1].onclick=()=>{modal.style.display='block';};
      modal.querySelector('button').onclick=()=>{choices[0].classList.remove('ovui-radio-item--checked');choices[1].classList.add('ovui-radio-item--checked');modal.style.display='none';};
    </script>
  `)
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = { id: 'scenario', page_kind: 'project_create', kind: 'field_action', field_key: 'project.application_scenario', operation: 'choose_exact_visible_option', scope: '营销目的', target: '应用调起', value: '应用调起', remote_write: false, blocked: false }
  await operations.applyField(step)
  expect(await operations.readField(step)).toBe('应用调起')
  await operations.applyField(step)
  expect(await operations.readField(step)).toBe('应用调起')
})

for (const sample of [
  { name: 'delayed saved URL', saved: 'https://example.test/landing', status: 'matched' },
  { name: 'different saved URL', saved: 'https://example.test/other', status: 'drifted' },
  { name: 'missing saved URL', saved: '', status: 'not_checked' },
  { name: 'saved URL after review', saved: 'https://example.test/landing', status: 'matched', reviewed: true },
]) {
  test(`Runner landing reconciliation handles ${sample.name}`, async ({ page, context }) => {
    test.setTimeout(45000)
    await context.route('https://oceanengine.test/**', route => route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: route.request().url().endsWith('/editor')
        ? `<input placeholder="请选择或填写自研落地页链接"><div data-e2e="createad_actionText__createRecommendTag"><span class="oc-tag-text">查看详情</span></div><script>setTimeout(() => document.querySelector('input').value = ${JSON.stringify(sample.saved)}, 500)</script>`
        : `<input placeholder="输入单元ID或名称后回车搜索"><table><tr class="ovui-tr"><td>测试单元 ID: 123456</td><td><div data-auto-id="promotion-status-card"><span class="oc-promotion-status-card-wrapper-title-value">未投放</span> ${sample.reviewed ? '账户余额不足' : '新建审核中'}</div></td><td><a href="/editor" target="_blank">编辑</a></td></tr></table>`,
    }))
    await page.goto('https://oceanengine.test/list')
    const plan = {
      plan_kind: 'promotion_create',
      steps: [
        { field_key: 'promotion.promotion_name', value: '测试单元' },
        { field_key: 'promotion.landing_page_reference', value: 'https://example.test/landing' },
        { field_key: 'promotion.call_to_action', value: ['查看详情'] },
      ],
    } as OceanEngineFormPlan
    const result = await new PlaywrightPageOperations(page).reconcileSubmit(plan, { outcome: 'result_unknown' })
    expect(result.created_object_id).toBe('123456')
    expect(result.platform_status).toBe(sample.reviewed ? 'not_delivering' : 'pending_review')
    expect(result.platform_status_text).toBe(sample.reviewed ? '未投放 账户余额不足' : '未投放 新建审核中')
    expect(result.field_reconciliation?.status).toBe(sample.status)
    expect(result.field_reconciliation?.fields.find(field => field.field_key === 'promotion.landing_page_reference')?.observed).toBe(sample.saved || undefined)
    expect(context.pages()).toHaveLength(1)
  })
}

test('Runner title fields preserve separate titles and replace the list on repeated Prepare', async ({ page }) => {
  await page.setContent(`
    <div data-e2e="createad_creativeTitles">
      <div class="creative-title-group-content"></div>
      <button id="add-title">点击添加</button>
    </div>
    <button id="add-landing-page">点击添加</button>
    <script>
      const group = document.querySelector('.creative-title-group-content');
      function addTitle() {
        const row = document.createElement('div');
        row.className = 'creative-title-item';
        row.innerHTML = '<div data-e2e="createad_creativeTitles__creativeTitleGroup_title_input_component"><input></div><button class="creative-title-item__delete">删除</button>';
        row.querySelector('button').onclick = () => setTimeout(() => row.remove(), 20);
        group.append(row);
      }
      document.querySelector('#add-title').onclick = () => setTimeout(addTitle, 20);
      document.querySelector('#add-landing-page').onclick = () => { throw new Error('wrong title group'); };
      addTitle();
    </script>
  `)
  const titles = ['{地点}{区县} 第一条独立标题', '{地点} 第二条独立标题{日期}']
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'copy', kind: 'field_action', field_key: 'promotion.copy_materials', operation: 'configure_object',
    scope: '文案素材', target: '请输入5-55个字的标题或输入关键词后选择推荐标题', value: titles,
    remote_write: false, blocked: false,
  }
  const operations = new PlaywrightPageOperations(page)
  await operations.applyField(step)
  expect(await operations.readField(step)).toEqual(titles)
  await operations.applyField(step)
  expect(await operations.readField(step)).toEqual(titles)
  await expect(page.locator('.creative-title-item')).toHaveCount(2)

  await operations.applyField({ ...step, value: [titles[1]] })
  expect(await operations.readField(step)).toEqual([titles[1]])
  await expect(page.locator('.creative-title-item')).toHaveCount(1)
  await page.locator('input').fill('人工修改后的独立标题')
  expect(await operations.readField(step)).toEqual(['人工修改后的独立标题'])
  await expect(operations.applyField({ ...step, value: [titles.join('\n')] })).rejects.toThrow('single-line')
  expect(await operations.readField(step)).toEqual(['人工修改后的独立标题'])
})

test('controlled execution center stays server-authoritative and safe on desktop and narrow layouts', async ({ page }) => {
  const runRoute = new RegExp(`/api/platform/v1/browser-rpa/projects/${projectId}/runs/${runId}(?:/(?:events|evidence))?$`)
  await page.route(runRoute, async route => {
    const url = new URL(route.request().url())
    if (route.request().method() !== 'GET') {
      await route.fulfill({ status: 405, json: { error: 'fake E2E does not authorize writes' } })
      return
    }
    if (url.pathname.endsWith('/events')) {
      await route.fulfill({ json: { items: [{ id: 'event_1', run_id: runId, sequence: 8, kind: 'result_reconciliation_required', summary: 'result unknown; query before any recovery', actor: 'fake-worker', created_at: '2026-08-12T10:00:00Z' }] } })
      return
    }
    if (url.pathname.endsWith('/evidence')) {
      await route.fulfill({ json: { items: [{ id: 'evidence_1', run_id: runId, step_id: 'submit_1', diff_keys: ['project_name', 'daily_budget'], redaction_version: 'browser-rpa-redaction/v1', selector_version: 'fake-selector/v1', created_at: '2026-08-12T10:00:00Z' }] } })
      return
    }
    await route.fulfill({ json: fakeRun() })
  })

  await page.route(new RegExp(`/api/platform/v1/browser-rpa/projects/${projectId}/(?:environments|browser-profiles|site-policies)/`), async route => {
    const url = route.request().url()
    const account_id = 'account_fake_1'
    const value = url.includes('/environments/')
      ? { id: 'environment_fake_1', account_id, mode: 'local_visible', browser_version: 'Edge', region: 'local', healthy: true, version: 1 }
      : url.includes('/browser-profiles/')
        ? { id: 'profile_fake_1', environment_id: 'environment_fake_1', account_id, state: 'ready', version: 1 }
        : { id: 'policy_fake_1', account_id, allowed_page_kinds: ['promotion_create'], allowed_platform_project_ids: [], version: 1 }
    await route.fulfill({ json: value })
  })

  await page.goto(`/projects/${projectId}/delivery/execution/${runId}`)
  const workspace = page.getByRole('region', { name: '受控执行中心' })
  await expect(workspace).toBeVisible()
  await expect(workspace.getByText('不要再次 Submit。先查询平台对象，再重新识别当前页面。必要时执行人工接管。确认平台结果后，创建新的补偿计划。')).toBeVisible()
  await expect(workspace.getByText('result unknown; query before any recovery')).toBeVisible()
  await expect(workspace.getByText('redaction=browser-rpa-redaction/v1 · selector=fake-selector/v1')).toBeVisible()
  await expect(workspace.getByRole('button', { name: /重试提交/ })).toHaveCount(0)
  await expect(workspace.getByText('approval_fake_1')).toBeVisible()
  await expect(workspace.getByRole('button', { name: '生成计划', exact: true })).toBeDisabled()
  await expect(workspace.getByRole('button', { name: '执行 Prepare', exact: true })).toBeDisabled()

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(workspace).toBeVisible()
  await expect(workspace.getByRole('button', { name: '从服务端刷新' })).toBeVisible()
  await expect(workspace.getByRole('button', { name: /取消运行/ })).toBeDisabled()
})

function fakeRun() {
  return {
    schema_version: 'browser-rpa-run/v1', id: runId, organization_id: 'org_local', project_id: projectId,
    platform: 'ocean_engine', account_id: 'account_fake_1',
    authority: {
      schema_version: 'browser-rpa-authority/v1', organization_id: 'org_local', project_id: projectId,
      business_execution_id: 'execution_fake_1', change_set_id: 'change_fake_1', approval_id: 'approval_fake_1', approval_action_hash: hash,
      account_reference_id: 'account_fake_1', object_fingerprint: hash, action: 'create_project_and_promotions', budget_limit_minor: 300000, currency: 'CNY',
      plan_canonical_hash: hash, intent_canonical_hash: hash, feedback_canonical_hash: hash, decision_canonical_hash: hash, configuration_canonical_hash: hash,
      workflow_id: 'workflow_fake_1', workflow_canonical_hash: hash, workflow_step_id: 'step_fake_1', skill_id: 'oceanengine-ecommerce-manual', skill_version: 'v0.1-calibration',
    },
    environment_id: 'environment_fake_1', profile_id: 'profile_fake_1', lease_id: 'lease_fake_1', policy_id: 'policy_fake_1',
    state: 'result_unknown', blocking_reason: 'RESULT_RECONCILIATION_REQUIRED', paused: false, takeover_active: false,
    version: 8, idempotency_key: 'fake-run-key', request_hash: hash, created_by: 'fake-e2e', created_at: '2026-08-12T10:00:00Z', updated_at: '2026-08-12T10:01:00Z',
  }
}
