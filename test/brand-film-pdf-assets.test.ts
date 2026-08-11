import assert from 'node:assert/strict'
import test from 'node:test'
import { briefProductNames, selectBriefProductMedia } from '../src/features/brand-film/pdfBriefAssets'
import type { ApiExtractedDocumentMedia } from '../src/data/api'

const media = (filename: string, page: number, width: number, height: number, pageText: string, mime: 'image/png' | 'image/jpeg' = 'image/png'): ApiExtractedDocumentMedia => ({
  filename, mime_type: mime, page_number: page, page_text: pageText, width, height,
  size_bytes: 100, sha256: filename, content: 'eA==',
})

test('PDF Brief keeps four named products and selects one portrait asset for each', () => {
  const brief = [
    '#娇兰第三代黄金复原蜜#',
    '法国娇兰对蜂蜜及其修护力逾10年专注研究的心血结晶',
    '产品词：娇兰第三代黄金复原蜜 or 娇兰第三代复原蜜；',
    '#25X蜂皇水#',
    '娇兰25X蜂皇水',
    '娇兰帝皇蜂姿面霜套组',
    '每天晚上，在娇兰黄金复原蜜之后',
    '#娇兰金钻修颜粉底液#',
  ].join('\n')
  assert.deepEqual(briefProductNames(brief), ['娇兰第三代黄金复原蜜', '25X蜂皇水', '娇兰帝皇蜂姿面霜套组', '娇兰金钻修颜粉底液'])
  const selected = selectBriefProductMedia(brief, [
    media('logo.png', 1, 229, 82, '封面'),
    media('honey.png', 4, 191, 513, '黄金复原蜜 成分与使用'),
    media('water-pump.png', 9, 189, 434, '25X蜂皇水'),
    media('water-front.png', 9, 98, 313, '25X蜂皇水'),
    media('cream.png', 11, 220, 360, '娇兰帝皇蜂姿面霜套组'),
    media('foundation.png', 19, 497, 988, '娇兰金钻修颜粉底液'),
  ])
  assert.equal(selected.filter(item => item.role === 'product_front').length, 4)
  assert.equal(selected.find(item => item.label === '25X蜂皇水')?.media.filename, 'water-front.png')
  assert.equal(selected.at(-1)?.role, 'logo')
})
