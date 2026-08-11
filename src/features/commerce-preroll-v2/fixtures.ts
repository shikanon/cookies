import type { FirstFrameCandidate, GeneratedPreroll, HookProposal, ProductFacts, ProductReference, RiskFact, SourceAnalysis, SourceVideo } from './types'

const assetRoot = '/assets/commerce-preroll-v2'
const guerlainRoot = '/assets'

export const fixtureSourceVideo: SourceVideo = {
  id: 'asset_guerlain_source_v1',
  name: '娇兰黄金复原蜜_电商主片.mp4',
  videoUrl: `${assetRoot}/source-guerlain.mp4`,
  posterUrl: `${guerlainRoot}/guerlain-youth-watery-oil-tail.jpg`,
  durationSeconds: 12,
  aspectRatio: '9:16',
  resolution: '720 × 1280',
  sizeLabel: '310 KB',
  version: 'v1',
  sourceLabel: '当前 Project 素材库',
  rightsStatus: 'confirmed',
}

export const fixtureProductFacts: ProductFacts = {
  name: '娇兰黄金复原蜜',
  category: '精华油 / 护肤',
  description: '琥珀金色瓶身，主打细腻润泽肤感与高端护肤体验。',
  sellingPoints: '轻盈油感、光泽肌观感、奢华金色质感',
  appearanceGuardrails: '保持金色瓶身比例、泵头结构、正面标签位置与字体布局；不可变更 Logo，不可新增包装文字。',
}

export const fixtureAnalysis: SourceAnalysis = {
  original: fixtureProductFacts,
  visualStyle: '暖金低饱和光影、中央稳定构图、浅景深商品特写',
  subtitleSummary: '强调轻盈肤感、光泽观感与高端护理体验',
  voiceSummary: '语速舒缓、克制，重点落在产品质感与使用体验',
  audioMood: '温暖、精致、低频稳定，无强节拍切换',
  openingShot: '正面产品中景，瓶身位于画面中央，背景为暗金光影',
  evidenceCount: 14,
}

export const fixtureProductReference: ProductReference = {
  id: 'reference_extracted_01',
  imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-tail.jpg`,
  label: '正面商品参考',
  sourceLabel: '从原视频 00:07.2 提取',
  kind: 'extracted',
}

export const fixtureRiskFacts: RiskFact[] = [
  { id: 'risk-effect-01', text: '改善细纹', sourceLabel: '来自画面字幕 00:08.4', status: 'pending' },
]

export const fixtureHooks: HookProposal[] = [
  {
    id: 'product_cut', name: '商品切割', imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-tail.jpg`,
    concept: '一道克制的切割光线掠过金色瓶身，外部材质被整齐分离，最终露出完整正面商品。',
    rationale: '原视频以中央产品特写为主，结构清晰，适合用几何切割快速建立视觉反差。',
    sellingPoint: '瓶身工艺 · 高端金色质感', action: '单一切割动作，不改变瓶型与标签。',
  },
  {
    id: 'frosted_window_reveal', name: '雾面橱窗揭幕', imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-frosted-start.jpg`,
    concept: '金色瓶身隐藏在雾面橱窗后，一次擦拭让轮廓、标签与瓶身光泽依次显现。',
    rationale: '原视频是暖金低饱和光影，中央构图稳定，适合模糊—清晰反差。',
    sellingPoint: '轻盈油感 · 光泽肌观感 · 高端金色质感', action: '只执行一次横向擦拭，最终正面定格。',
  },
  {
    id: 'one_click_grab', name: '一键取物', imageUrl: `${guerlainRoot}/guerlain-25x-bee-water-product-front.png`,
    concept: '手指点击画面中央的金色光点，商品从暖金背景中被轻柔取出并稳定悬停。',
    rationale: '原视频商品轮廓明确，交互式动作能在短时间内强化产品出现的瞬间。',
    sellingPoint: '轻盈触感 · 精致使用体验', action: '一次点击与一次取物，不出现多余手部动作。',
  },
  {
    id: 'miniature_effect_stage', name: '微缩功效剧场', imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-tail.jpg`,
    concept: '微缩金色水滴沿瓶身周围形成润泽轨迹，最后汇聚到商品正面形成光泽定格。',
    rationale: '原视频强调润泽肤感，微缩视觉可把抽象卖点转成可理解的动作。',
    sellingPoint: '润泽感 · 光泽肌观感', action: '只表现润泽氛围，不制造未经确认的功效数据。',
  },
  {
    id: 'device_recall', name: '3C 设备召回', imageUrl: `${guerlainRoot}/guerlain-25x-bee-water-product-front.png`,
    concept: '商品像被展示装置召回一样从暗处沿垂直光轨进入画面中央，最终完成正面锁定。',
    rationale: '稳定的中央构图适合装置式召回，可制造更强的开场控制感。',
    sellingPoint: '高端陈列感 · 识别度', action: '垂直召回一次，保持瓶体比例与正面标签。',
  },
]

export const fixtureFirstFrames: FirstFrameCandidate[] = [
  { id: 'frame-01', imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-frosted-start.jpg`, label: '方案一 · 克制', title: '雾面轮廓', description: '商品保持不可见细节，后续擦拭反差最强。' },
  { id: 'frame-02', imageUrl: `${guerlainRoot}/guerlain-youth-watery-oil-tail.jpg`, label: '方案二 · 通透', title: '蜂巢暖金', description: '复用原视频暖金光影与中央商品构图。' },
  { id: 'frame-03', imageUrl: `${guerlainRoot}/guerlain-25x-bee-water-product-front.png`, label: '方案三 · 保真', title: '正面瓶身', description: '正面标签与瓶型最清晰，优先保证商品识别。' },
]

export function fixtureOutput(duration: GeneratedPreroll['duration']): GeneratedPreroll {
  return {
    id: `generated_preroll_${Date.now()}`,
    videoUrl: `${assetRoot}/result-window-reveal-${duration}s.mp4`,
    posterUrl: `${guerlainRoot}/guerlain-youth-watery-oil-frosted-start.jpg`,
    duration,
    aspectRatio: '9:16',
    createdAt: new Date().toISOString(),
  }
}
