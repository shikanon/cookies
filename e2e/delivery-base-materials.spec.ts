import { expect, test } from '@playwright/test'
import { PlaywrightPageOperations } from '../scripts/browser-rpa-runner-v3'
import type { OceanEngineFormPlan } from '../scripts/oceanengine-form-plan-compiler'

test('mixed base materials survive repeated preparation and reject a changed selection', async ({ page }) => {
  test.setTimeout(120_000)
  await page.setContent(`
    <button id="video">添加视频</button><button id="image">添加图片</button>
    <div data-e2e="createad_materialSelected_video">视频(0/30)</div>
    <div data-e2e="createad_materialSelected_image">图片(0/50)</div>
    <div data-e2e="createad_materialSelected_awemePhoto">图文(0/10)</div>
    <div role="dialog" style="display:none"><input id="search"><input placeholder="可搜索视频名称或ID" style="display:none"><div id="cards"></div><span id="count"></span>
      <button id="cancel">取消</button><button id="confirm">确定</button></div>
    <button id="replace">替换为未要求的视频</button>
    <script>
      const inventory = {video: [['101','视频甲'],['102','视频乙'],['103','其他视频']],image:[['201','图片甲'],['202','图片乙']]};
      const saved = {video: new Set(), image: new Set()};
      let kind, selected;
      const dialog=document.querySelector('[role="dialog"]'), search=document.querySelector('#search');
      function render() {
        document.querySelector('#cards').innerHTML=inventory[kind].filter(([id])=>!search.value||id===search.value)
          .map(([id,label])=>'<div class="create-material-list-card-item"><span>'+label+'</span><input type="checkbox" data-id="'+id+'" '+(selected.has(id)?'checked':'')+'></div>').join('');
        document.querySelectorAll('[data-id]').forEach(input=>input.onchange=()=>{input.checked?selected.add(input.dataset.id):selected.delete(input.dataset.id); count()});
        count();
      }
      function count(){document.querySelector('#count').textContent='已选择'+selected.size+'/10：'}
      for(const type of ['video','image']) document.querySelector('#'+type).onclick=()=>{
        kind=type; selected=new Set(saved[type]); search.value='';
        search.placeholder=type==='video'?'可搜索视频名称或ID':'请输入图片名称或ID'; dialog.style.display='block'; render();
      };
      search.oninput=render;
      document.querySelector('#cancel').onclick=()=>dialog.style.display='none';
      document.querySelector('#confirm').onclick=()=>{
        saved[kind]=new Set(selected); dialog.style.display='none';
        document.querySelector('[data-e2e="createad_materialSelected_'+kind+'"]').textContent=(kind==='video'?'视频':'图片')+'('+saved[kind].size+'/'+(kind==='video'?30:50)+')';
      };
      document.querySelector('#replace').onclick=()=>{saved.video.delete('102');saved.video.add('103')};
    </script>`)
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'base-materials', kind: 'field_action', field_key: 'promotion.base_materials',
    operation: 'open_reference_picker', remote_write: false, blocked: false,
    value: [
      { selection_kind: 'async_row', material_type: 'video_material', object_id: '101', label: '视频甲' },
      { selection_kind: 'async_row', material_type: 'video_material', object_id: '102', label: '视频乙' },
      { selection_kind: 'async_row', material_type: 'image_material', object_id: '201', label: '图片甲' },
      { selection_kind: 'async_row', material_type: 'image_material', object_id: '202', label: '图片乙' },
    ],
  }
  await operations.applyField(step)
  await operations.applyField(step)
  const readback = await operations.readField(step) as Array<{ object_id: string }>
  expect(readback.map(item => item.object_id)).toEqual(['101', '102', '201', '202'])
  await expect(page.getByText('视频(2/30)', { exact: true })).toBeVisible()
  await expect(page.getByText('图片(2/50)', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '替换为未要求的视频' }).click()
  await expect(operations.readField(step)).rejects.toThrow('base material 102 is not selected')
})

test('landing page picker searches by ID before selecting beyond the initial page', async ({ page }) => {
  await page.setContent(`<span>落地页</span><input placeholder="请选择橙子落地页链接"><button class="input__suffix">选择</button>
    <div role="dialog" style="display:none"><input placeholder="请输入名称关键词或ID"><input placeholder="请输入名称关键词或ID" style="display:none">
    <div id="cards"><div class="create-material-list-card-item">其他页面<span>ID: 999</span></div></div>
    <span id="selected">已选择0/1</span><button id="confirm">确定</button></div>
    <script>
      const dialog=document.querySelector('[role="dialog"]');
      document.querySelector('.input__suffix').onclick=()=>dialog.style.display='block';
      document.querySelector('[placeholder="请输入名称关键词或ID"]').onkeydown=e=>{
        if(e.key==='Enter'&&e.target.value==='123'){
          document.querySelector('#cards').innerHTML='<div class="create-material-list-card-item">目标落地页<span>ID: 123</span></div>';
          document.querySelector('#cards').onclick=()=>document.querySelector('#selected').textContent='已选择1/1';
        }
      };
      document.querySelector('#confirm').onclick=()=>dialog.style.display='none';
    </script>`)
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = { id: 'landing', kind: 'field_action', field_key: 'promotion.landing_page_reference', operation: 'open_reference_picker', scope: '落地页', target: '请选择橙子落地页链接', remote_write: false, blocked: false, value: { selection_kind: 'async_row', object_id: '123', label: '目标落地页' } }
  await operations.applyField(step)
  expect(await operations.readField(step)).toMatchObject({ object_id: '123', selected_count: 1 })
  await expect(page.getByRole('dialog')).toBeHidden()
})

test('duplicate or untyped base materials stop before opening a picker', async ({ page }) => {
  await page.setContent('<p>尚未打开素材库</p>')
  const operations = new PlaywrightPageOperations(page)
  const spec = { selection_kind: 'async_row', material_type: 'video_material', object_id: '101', label: '视频甲' }
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'base-materials', kind: 'field_action', field_key: 'promotion.base_materials',
    operation: 'open_reference_picker', remote_write: false, blocked: false, value: [spec, spec],
  }
  await expect(operations.applyField(step)).rejects.toThrow('duplicate base material')
  await expect(operations.applyField({ ...step, value: [{ ...spec, material_type: undefined }] })).rejects.toThrow('typed video or image IDs')
})

test('project budget mode ignores unrelated unlimited choices and reads the selected mode', async ({ page }) => {
  await page.setContent(`<div id="region">不限</div><div data-e2e="createproject_budgettypeselect">
    <div class="ovui-radio-item ovui-radio-item--checked">不限</div><div class="ovui-radio-item">设置预算</div></div>
    <script>document.querySelectorAll('.ovui-radio-item').forEach(option=>option.onclick=()=>{
      document.querySelectorAll('.ovui-radio-item').forEach(item=>item.classList.remove('ovui-radio-item--checked'));
      option.classList.add('ovui-radio-item--checked');
    })</script>`)
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'budget-mode', kind: 'field_action', field_key: 'project.budget_mode', operation: 'choose_exact_visible_option',
    target: '不限', value: '设置预算', remote_write: false, blocked: false,
  }
  await operations.applyField(step)
  expect(await operations.readField(step)).toBe('设置预算')
  await page.locator('[data-e2e="createproject_budgettypeselect"]').getByText('不限', { exact: true }).click()
  await expect(operations.readField(step)).rejects.toThrow('budget mode differs from the plan')
})

test('a missing bid input cannot overwrite the daily budget', async ({ page }) => {
  await page.setContent('<div class="oc-row"><span>日预算</span><input role="spinbutton" value="300"></div><div class="oc-row"><span>出价</span></div>')
  const operations = new PlaywrightPageOperations(page)
  const step: OceanEngineFormPlan['steps'][number] = {
    id: 'project-bid', kind: 'field_action', field_key: 'project.bid', operation: 'fill_money',
    scope: '出价', target: 'spinbutton', value: '0.01', remote_write: false, blocked: false,
  }
  await expect(operations.applyField(step)).rejects.toThrow('bid input is unavailable in its own field row')
  await expect(page.getByRole('spinbutton')).toHaveValue('300')
  await page.setContent('<div class="oc-row"><span>日预算</span><input role="spinbutton" value="300"></div><div class="oc-row"><span>出价</span><input role="spinbutton" value="1"></div>')
  await operations.applyField(step)
  await expect(page.getByRole('spinbutton').nth(0)).toHaveValue('300')
  await expect(page.getByRole('spinbutton').nth(1)).toHaveValue('0.01')
  await page.setContent('<div class="oc-row"><span>单元预算</span><input type="number" value="300"></div><div class="b-row"><span>单元出价</span><div class="input-group-wrap"><input type="number" value="1"></div></div>')
  await operations.applyField({ ...step, id: 'promotion-bid', field_key: 'promotion.bid', scope: '单元出价' })
  await expect(page.getByRole('spinbutton').nth(0)).toHaveValue('300')
  await expect(page.getByRole('spinbutton').nth(1)).toHaveValue('0.01')
})
