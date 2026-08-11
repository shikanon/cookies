import { api, type ApiBrandBriefAssetCandidate, type ApiExtractedDocumentMedia, type ApiKnowledgeDocument } from '../../data/api'

const productKeywords = ['蜜', '水', '精华', '面霜', '乳霜', '粉底', '口红', '唇膏', '香水', '眼霜', '面膜', '套组']
const normalize = (value: string) => value.replace(/\s+/g, '').replace(/[#[\]【】（）()：:，,。.;；]/g, '').toLowerCase()

export function briefProductNames(briefText: string): string[] {
  const result: string[] = []
  const add = (raw: string) => {
    const value = raw.trim().replace(/^[#\s]+|[#\s]+$/g, '')
    if (
      !value
      || [...value].length > 20
      || /[\s，,。.;；：:、“”'‘’/]/.test(value)
      || !productKeywords.some(keyword => value.includes(keyword))
    ) return
    const identity = normalize(value).replace(/^法国娇兰|^娇兰/, '')
    if (!result.some(current => normalize(current).replace(/^法国娇兰|^娇兰/, '') === identity)) result.push(value)
  }
  for (const line of briefText.replace(/\r\n/g, '\n').split('\n')) {
    const hashtags = [...line.matchAll(/#\s*([^#\r\n]{2,48}?)\s*#/g)]
    if (hashtags.length) hashtags.forEach(match => add(match[1]))
    else if (line.includes('娇兰')) add(line)
  }
  return result
}

const portraitScore = (media: ApiExtractedDocumentMedia, titlePage?: number) => {
  if (media.width < 70 || media.height < 170 || media.height / Math.max(media.width, 1) < 1.12) return Number.NEGATIVE_INFINITY
  const ratio = media.height / Math.max(media.width, 1)
  const area = media.width * media.height
  const pageAffinity = titlePage ? 260 - Math.abs(media.page_number - (titlePage + 1)) * 90 : 0
  return ratio * 150 + Math.min(area / 2500, 180) + (media.mime_type === 'image/png' ? 70 : 0) + pageAffinity
}

export function selectBriefProductMedia(briefText: string, media: ApiExtractedDocumentMedia[]) {
  const products = briefProductNames(briefText)
  const used = new Set<string>()
  const selected: Array<{ label: string; role: 'product_front' | 'logo'; media: ApiExtractedDocumentMedia }> = []
  for (const product of products) {
    const productToken = normalize(product).replace(/^法国娇兰|^娇兰/, '').replace(/^第[一二三四五六七八九十0-9]+代/, '')
    const titlePage = media.find(item => {
      const pageToken = normalize(item.page_text ?? '')
      return pageToken.includes(normalize(product)) || (productToken.length >= 4 && pageToken.includes(productToken))
    })?.page_number
    const ranked = media
      .filter(item => !used.has(item.sha256) && (!titlePage || (item.page_number >= titlePage && item.page_number <= titlePage + 2)))
      .map(item => ({ item, score: portraitScore(item, titlePage) }))
      .filter(item => Number.isFinite(item.score))
      .sort((left, right) => right.score - left.score)
    const fallback = media
      .filter(item => !used.has(item.sha256))
      .map(item => ({ item, score: portraitScore(item) }))
      .filter(item => Number.isFinite(item.score))
      .sort((left, right) => right.score - left.score)
    const choice = (ranked[0] ?? fallback[0])?.item
    if (!choice) continue
    used.add(choice.sha256)
    selected.push({ label: product, role: 'product_front', media: choice })
  }
  const logo = media
    .filter(item => !used.has(item.sha256) && item.page_number <= 2 && item.width >= 100 && item.height >= 40 && item.width / Math.max(item.height, 1) >= 1.8)
    .map(item => ({ item, score: (item.mime_type === 'image/png' ? 500 : 0) - item.width * item.height / 10_000 }))
    .sort((left, right) => right.score - left.score)[0]?.item
  if (logo) selected.push({ label: '品牌 Logo', role: 'logo', media: logo })
  return selected
}

const mediaFile = (media: ApiExtractedDocumentMedia, label: string) => {
  const binary = window.atob(media.content)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index)
  const extension = media.mime_type === 'image/png' ? 'png' : 'jpg'
  const safeLabel = label.replace(/[\\/:*?"<>|]/g, '-').slice(0, 48) || 'brief-image'
  return new File([bytes], `${safeLabel}.${extension}`, { type: media.mime_type })
}

export async function extractAndUploadBrandBriefAssets(projectId: string, document: ApiKnowledgeDocument): Promise<ApiBrandBriefAssetCandidate[]> {
  if (document.mime_type !== 'application/pdf' || !document.extracted_text) return []
  const extraction = await api.extractKnowledgeDocumentMedia(projectId, document.id)
  const selected = selectBriefProductMedia(document.extracted_text, extraction.items)
  return Promise.all(selected.map(async (selection, index) => {
    const assetRef = await api.uploadProjectAsset(projectId, mediaFile(selection.media, selection.label))
    return {
      id: selection.role === 'logo' ? 'asset_brand_logo' : `asset_product_front_${String(index + 1).padStart(2, '0')}`,
      role: selection.role,
      label: selection.label,
      source_locator: `knowledge://documents/${document.id}#page=${selection.media.page_number}&image=${encodeURIComponent(selection.media.filename)}`,
      asset_ref: assetRef,
      rights_status: 'needs_confirmation',
      user_confirmed: false,
      replacement_note: `从 ${document.filename ?? document.title} 第 ${selection.media.page_number} 页提取`,
    }
  }))
}
