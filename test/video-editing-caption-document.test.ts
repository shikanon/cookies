import assert from 'node:assert/strict'
import test from 'node:test'

import {
  createCaptionDocument,
  deleteCaption,
  importSubtitleText,
  mergeCaptionWithNext,
  splitCaption,
  updateCaption,
} from '../src/features/video-editing/captionDocument'

test('C4 imports SRT and ASS into frame-aligned caption drafts', () => {
  const srt = importSubtitleText('intro.srt', '\uFEFF1\r\n00:00:01,000 --> 00:00:02,500\r\n你好，Cookies！\r\n\r\n2\r\n00:00:02,500 --> 00:00:04,000\r\nEnglish 123。\r\n')
  assert.equal(srt.format, 'srt')
  assert.deepEqual(srt.captions.map(item => [item.timelineStartMs, item.timelineEndMs, item.text]), [[1000, 2500, '你好，Cookies！'], [2500, 4000, 'English 123。']])

  const ass = importSubtitleText('intro.ass', '[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:03.20,Default,,0,0,0,,第一行\\N第二行')
  assert.equal(ass.format, 'ass')
  assert.equal(ass.captions[0].text, '第一行\n第二行')
  assert.equal(ass.captions[0].timelineEndMs, 3200)
})

test('C4 caption edits are immutable and support split merge move trim and emphasis', () => {
  const imported = importSubtitleText('captions.srt', '1\n00:00:00,000 --> 00:00:02,000\n品牌关键词\n\n2\n00:00:02,000 --> 00:00:04,000\n第二句\n')
  const base = createCaptionDocument(imported.captions)
  const edited = updateCaption(base, base.captions[0].id, { text: '品牌 关键词', timelineStartMs: 500, timelineEndMs: 2500, emphasis: [{ start: 3, end: 6 }] })
  assert.equal(base.captions[0].text, '品牌关键词')
  assert.deepEqual(edited.captions[0].emphasis, [{ start: 3, end: 6 }])

  const split = splitCaption(edited, edited.captions[0].id, 1500, 3)
  assert.equal(split.captions.length, 3)
  assert.deepEqual(split.captions.slice(0, 2).map(item => [item.timelineStartMs, item.timelineEndMs]), [[500, 1500], [1500, 2500]])
  const merged = mergeCaptionWithNext(split, split.captions[0].id)
  assert.equal(merged.captions.length, 2)
  assert.equal(merged.captions[0].text, '品牌 关键词')
  assert.equal(deleteCaption(merged, merged.captions[0].id).captions.length, 1)
})

test('C4 rejects malformed imports instead of inventing captions', () => {
  assert.throws(() => importSubtitleText('captions.srt', 'not a subtitle'), /没有可导入的字幕/)
  assert.throws(() => importSubtitleText('captions.txt', 'hello'), /仅支持 SRT 或 ASS/)
})
