export type CaptionEmphasis = { start: number; end: number }

export type CaptionClip = {
  id: string
  text: string
  timelineStartMs: number
  timelineEndMs: number
  styleId: string
  styleVersion: number
  emphasis: CaptionEmphasis[]
}

export type CaptionDocument = {
  trackId: string
  language: string
  captions: CaptionClip[]
}

export type SubtitleImport = {
  format: 'srt' | 'ass'
  captions: CaptionClip[]
  diagnostics: string[]
}

const DEFAULT_STYLE = { styleId: 'brand-default', styleVersion: 1 }

export function createCaptionDocument(captions: CaptionClip[] = [], language = 'zh-CN', trackId = 'captions-main'): CaptionDocument {
  return normalizeCaptionDocument({ trackId, language, captions: captions.map(caption => ({ ...caption, emphasis: caption.emphasis.map(span => ({ ...span })) })) })
}

export function importSubtitleText(fileName: string, source: string): SubtitleImport {
  const extension = fileName.trim().toLowerCase().split('.').pop()
  if (extension !== 'srt' && extension !== 'ass') throw new Error('仅支持 SRT 或 ASS 字幕文件')
  const text = source.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const values = extension === 'srt' ? parseSRT(text) : parseASS(text)
  if (!values.length) throw new Error('没有可导入的字幕，请检查时间码和文本')
  return {
    format: extension,
    captions: values.map((value, index) => ({ id: `caption-import-${index + 1}`, ...value, ...DEFAULT_STYLE, emphasis: [] })),
    diagnostics: [],
  }
}

export function updateCaption(document: CaptionDocument, id: string, patch: Partial<Omit<CaptionClip, 'id'>>): CaptionDocument {
  return normalizeCaptionDocument({
    ...document,
    captions: document.captions.map(caption => caption.id === id ? sanitizeCaption({ ...caption, ...patch, emphasis: patch.emphasis?.map(span => ({ ...span })) ?? caption.emphasis }) : caption),
  })
}

export function deleteCaption(document: CaptionDocument, id: string): CaptionDocument {
  return normalizeCaptionDocument({ ...document, captions: document.captions.filter(caption => caption.id !== id) })
}

export function splitCaption(document: CaptionDocument, id: string, atMs: number, textRuneIndex: number): CaptionDocument {
  const caption = document.captions.find(item => item.id === id)
  const splitMS = alignFrame(atMs)
  const runes = [...(caption?.text ?? '')]
  if (!caption || splitMS <= caption.timelineStartMs || splitMS >= caption.timelineEndMs || textRuneIndex <= 0 || textRuneIndex >= runes.length) return document
  const ordinal = nextOrdinal(document, `${id}-split`)
  const leftText = runes.slice(0, textRuneIndex).join('').trimEnd()
  const rightText = runes.slice(textRuneIndex).join('').trimStart()
  if (!leftText || !rightText) return document
  const left: CaptionClip = { ...caption, id: `${id}-split-${ordinal}-a`, text: leftText, timelineEndMs: splitMS, emphasis: [] }
  const right: CaptionClip = { ...caption, id: `${id}-split-${ordinal}-b`, text: rightText, timelineStartMs: splitMS, emphasis: [] }
  return normalizeCaptionDocument({ ...document, captions: document.captions.flatMap(item => item.id === id ? [left, right] : [item]) })
}

export function mergeCaptionWithNext(document: CaptionDocument, id: string): CaptionDocument {
  const sorted = [...document.captions].sort(compareCaptions)
  const index = sorted.findIndex(item => item.id === id)
  if (index < 0 || index + 1 >= sorted.length) return document
  const left = sorted[index]
  const right = sorted[index + 1]
  if (left.styleId !== right.styleId || left.styleVersion !== right.styleVersion) return document
  const separator = /\s$/u.test(left.text) || /^\s/u.test(right.text) ? '' : ' '
  const merged: CaptionClip = { ...left, text: `${left.text}${separator}${right.text}`, timelineEndMs: right.timelineEndMs, emphasis: [] }
  return normalizeCaptionDocument({ ...document, captions: document.captions.filter(item => item.id !== right.id).map(item => item.id === left.id ? merged : item) })
}

function parseSRT(source: string): Array<Pick<CaptionClip, 'timelineStartMs' | 'timelineEndMs' | 'text'>> {
  const result: Array<Pick<CaptionClip, 'timelineStartMs' | 'timelineEndMs' | 'text'>> = []
  for (const block of source.split(/\n{2,}/u)) {
    const lines = block.split('\n').map(line => line.trimEnd()).filter((line, index) => index > 0 || line.trim())
    const timingIndex = lines.findIndex(line => line.includes('-->'))
    if (timingIndex < 0) continue
    const [startRaw, endRaw] = lines[timingIndex].split('-->').map(value => value.trim().split(/\s+/u)[0])
    const start = parseTime(startRaw)
    const end = parseTime(endRaw)
    const text = lines.slice(timingIndex + 1).join('\n').trim()
    if (start !== undefined && end !== undefined && end > start && text) result.push({ timelineStartMs: alignFrame(start), timelineEndMs: alignFrame(end), text })
  }
  return result
}

function parseASS(source: string): Array<Pick<CaptionClip, 'timelineStartMs' | 'timelineEndMs' | 'text'>> {
  const result: Array<Pick<CaptionClip, 'timelineStartMs' | 'timelineEndMs' | 'text'>> = []
  for (const line of source.split('\n')) {
    if (!/^Dialogue\s*:/iu.test(line)) continue
    const fields = line.replace(/^Dialogue\s*:\s*/iu, '').split(',')
    if (fields.length < 10) continue
    const start = parseTime(fields[1])
    const end = parseTime(fields[2])
    const text = fields.slice(9).join(',').replace(/\{[^}]*\}/gu, '').replace(/\\[Nn]/gu, '\n').replace(/\\h/gu, ' ').trim()
    if (start !== undefined && end !== undefined && end > start && text) result.push({ timelineStartMs: alignFrame(start), timelineEndMs: alignFrame(end), text })
  }
  return result
}

function parseTime(value: string | undefined): number | undefined {
  const match = value?.trim().match(/^(\d+):(\d{2}):(\d{2})[,.](\d{1,3})$/u)
  if (!match) return undefined
  const fraction = match[4].padEnd(3, '0').slice(0, 3)
  return Number(match[1]) * 3_600_000 + Number(match[2]) * 60_000 + Number(match[3]) * 1000 + Number(fraction)
}

function sanitizeCaption(caption: CaptionClip): CaptionClip {
  const text = caption.text.trim()
  const start = alignFrame(caption.timelineStartMs)
  const end = Math.max(start + 33, alignFrame(caption.timelineEndMs))
  const runeCount = [...text].length
  const emphasis = caption.emphasis.filter(span => Number.isInteger(span.start) && Number.isInteger(span.end) && span.start >= 0 && span.end > span.start && span.end <= runeCount)
  return { ...caption, text, timelineStartMs: start, timelineEndMs: end, emphasis }
}

function normalizeCaptionDocument(document: CaptionDocument): CaptionDocument {
  return { ...document, captions: document.captions.map(sanitizeCaption).filter(caption => caption.text).sort(compareCaptions) }
}

function compareCaptions(left: CaptionClip, right: CaptionClip) {
  return left.timelineStartMs - right.timelineStartMs || left.id.localeCompare(right.id)
}

function nextOrdinal(document: CaptionDocument, prefix: string) {
  return document.captions.filter(caption => caption.id.startsWith(prefix)).length + 1
}

function alignFrame(ms: number): number {
  return Math.max(0, Math.round(Math.round(ms * 30 / 1000) * 1000 / 30))
}
