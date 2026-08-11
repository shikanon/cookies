export type AudioRole = 'voiceover' | 'music' | 'sfx'

export type AudioAsset = {
  kind: 'audio'
  assetId: string
  assetVersion: number
  durationMs: number
  name: string
  previewUrl: string
  waveformPeaks: number[]
}

export type AudioClip = AudioAsset & {
  id: string
  trackId: string
  timelineStartMs: number
  timelineEndMs: number
  sourceInMs: number
  sourceOutMs: number
  gainDb: number
  fadeInMs: number
  fadeOutMs: number
  loop: boolean
}

export type AudioTrack = { id: string; role: AudioRole; muted: boolean; clips: AudioClip[] }
export type AudioDocument = { tracks: [AudioTrack, AudioTrack, AudioTrack] }

export const C5_AUDIO_TRACKS = ['audio-voiceover', 'audio-music', 'audio-sfx'] as const

export function createEmptyAudioDocument(): AudioDocument {
  return { tracks: [
    { id: C5_AUDIO_TRACKS[0], role: 'voiceover', muted: false, clips: [] },
    { id: C5_AUDIO_TRACKS[1], role: 'music', muted: false, clips: [] },
    { id: C5_AUDIO_TRACKS[2], role: 'sfx', muted: false, clips: [] },
  ] }
}

export function insertAudioAsset(document: AudioDocument, asset: AudioAsset, trackId: string, atMs: number): AudioDocument {
  const track = document.tracks.find(item => item.id === trackId)
  if (!track || asset.durationMs <= 0) return document
  const start = alignFrame(atMs)
  const ordinal = audioClips(document).filter(clip => clip.assetId === asset.assetId && clip.assetVersion === asset.assetVersion).length + 1
  const clip: AudioClip = { ...asset, id: `audio-${asset.assetId}-v${asset.assetVersion}-${ordinal}`, trackId, timelineStartMs: start, timelineEndMs: start + alignFrame(asset.durationMs), sourceInMs: 0, sourceOutMs: alignFrame(asset.durationMs), gainDb: track.role === 'music' ? -12 : 0, fadeInMs: 0, fadeOutMs: 0, loop: false }
  return normalize({ tracks: document.tracks.map(item => item.id === trackId ? { ...item, clips: [...item.clips, clip] } : item) as AudioDocument['tracks'] })
}

export function moveAudioClip(document: AudioDocument, clipId: string, targetTrackId: string, atMs: number): AudioDocument {
  const source = document.tracks.find(track => track.clips.some(clip => clip.id === clipId))
  const target = document.tracks.find(track => track.id === targetTrackId)
  const clip = source?.clips.find(item => item.id === clipId)
  if (!source || !target || !clip) return document
  const start = alignFrame(atMs)
  const moved = { ...clip, trackId: target.id, timelineStartMs: start, timelineEndMs: start + clip.timelineEndMs - clip.timelineStartMs }
  return normalize({ tracks: document.tracks.map(track => ({ ...track, clips: track.id === target.id ? [...track.clips.filter(item => item.id !== clipId), moved] : track.clips.filter(item => item.id !== clipId) })) as AudioDocument['tracks'] })
}

export function trimAudioClip(document: AudioDocument, clipId: string, timelineStartMs: number, timelineEndMs: number, sourceInMs: number, sourceOutMs: number): AudioDocument {
  if (timelineStartMs < 0 || timelineEndMs <= timelineStartMs || sourceInMs < 0 || sourceOutMs <= sourceInMs) return document
  return mapClip(document, clipId, clip => sourceOutMs > clip.durationMs ? clip : { ...clip, timelineStartMs: alignFrame(timelineStartMs), timelineEndMs: alignFrame(timelineEndMs), sourceInMs: alignFrame(sourceInMs), sourceOutMs: alignFrame(sourceOutMs) })
}

export function splitAudioClip(document: AudioDocument, clipId: string, atMs: number): AudioDocument {
  const track = document.tracks.find(item => item.clips.some(clip => clip.id === clipId))
  const clip = track?.clips.find(item => item.id === clipId)
  const split = alignFrame(atMs)
  if (!track || !clip || split <= clip.timelineStartMs || split >= clip.timelineEndMs) return document
  const sourceSplit = clip.sourceInMs + split - clip.timelineStartMs
  const left = { ...clip, id: `${clip.id}-a`, timelineEndMs: split, sourceOutMs: sourceSplit, fadeOutMs: 0 }
  const right = { ...clip, id: `${clip.id}-b`, timelineStartMs: split, sourceInMs: sourceSplit, fadeInMs: 0 }
  return normalize({ tracks: document.tracks.map(item => item.id === track.id ? { ...item, clips: item.clips.flatMap(value => value.id === clipId ? [left, right] : [value]) } : item) as AudioDocument['tracks'] })
}

export function updateAudioClip(document: AudioDocument, clipId: string, patch: Partial<Pick<AudioClip, 'gainDb' | 'fadeInMs' | 'fadeOutMs' | 'loop'>>): AudioDocument {
  return mapClip(document, clipId, clip => {
    const duration = clip.timelineEndMs - clip.timelineStartMs
    return { ...clip, gainDb: clamp(patch.gainDb ?? clip.gainDb, -96, 24), fadeInMs: clamp(alignFrame(patch.fadeInMs ?? clip.fadeInMs), 0, duration), fadeOutMs: clamp(alignFrame(patch.fadeOutMs ?? clip.fadeOutMs), 0, duration), loop: patch.loop ?? clip.loop }
  })
}

export function deleteAudioClip(document: AudioDocument, clipId: string): AudioDocument {
  return normalize({ tracks: document.tracks.map(track => ({ ...track, clips: track.clips.filter(clip => clip.id !== clipId) })) as AudioDocument['tracks'] })
}

export function setAudioTrackMuted(document: AudioDocument, trackId: string, muted: boolean): AudioDocument {
  return { tracks: document.tracks.map(track => track.id === trackId ? { ...track, muted } : track) as AudioDocument['tracks'] }
}

export function audioClips(document: AudioDocument) { return document.tracks.flatMap(track => track.clips) }

function mapClip(document: AudioDocument, clipId: string, update: (clip: AudioClip) => AudioClip): AudioDocument {
  if (!document.tracks.some(track => track.clips.some(clip => clip.id === clipId))) return document
  return normalize({ tracks: document.tracks.map(track => ({ ...track, clips: track.clips.map(clip => clip.id === clipId ? update(clip) : clip) })) as AudioDocument['tracks'] })
}

function normalize(document: AudioDocument): AudioDocument {
  return { tracks: document.tracks.map(track => ({ ...track, clips: [...track.clips].sort((a, b) => a.timelineStartMs - b.timelineStartMs || a.id.localeCompare(b.id)) })) as AudioDocument['tracks'] }
}

function alignFrame(ms: number) { return Math.max(0, Math.round(Math.round(ms * 30 / 1000) * 1000 / 30)) }
function clamp(value: number, min: number, max: number) { return Math.min(max, Math.max(min, Number.isFinite(value) ? value : min)) }
