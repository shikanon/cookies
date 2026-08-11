import { performance } from 'node:perf_hooks'

import { createEmptyVisualDocument, insertVisualAsset, moveVisualClip, visualClips } from '../src/features/video-editing/visualTimeline.ts'

const results = [10, 50, 100].map(clipCount => benchmark(clipCount))
for (const result of results) {
  if (result.p95Ms > 5 || result.maxMs > 50 || result.heapDeltaMB > 64) {
    throw new Error(`timeline performance budget exceeded for ${result.clipCount} clips: ${JSON.stringify(result)}`)
  }
}
process.stdout.write(`${JSON.stringify({ runtime: 'C3 visual timeline immutable pointer-update kernel', results }, null, 2)}\n`)

function benchmark(clipCount: number) {
  let document = createEmptyVisualDocument()
  for (let index = 0; index < clipCount; index += 1) {
    document = insertVisualAsset(document, {
      assetId: `benchmark-${index}`, assetVersion: 1, kind: index % 4 === 0 ? 'image' : 'video', durationMs: 1000,
      name: `Benchmark ${index}`, previewUrl: '',
    }, document.tracks[index % 3].id, index * 350)
  }
  const beforeHeap = process.memoryUsage().heapUsed
  const samples: number[] = []
  for (let index = 0; index < 2_000; index += 1) {
    const clips = visualClips(document)
    const clip = clips[index % clips.length]
    const target = document.tracks[(index + 1) % 3]
    const started = performance.now()
    document = moveVisualClip(document, clip.id, target.id, (index * 37) % Math.max(1, document.durationMs))
    samples.push(performance.now() - started)
  }
  samples.sort((left, right) => left - right)
  const percentile = (value: number) => samples[Math.min(samples.length - 1, Math.floor(samples.length * value))]
  return { clipCount, iterations: samples.length, p50Ms: Number(percentile(0.5).toFixed(4)), p95Ms: Number(percentile(0.95).toFixed(4)), maxMs: Number(samples.at(-1)?.toFixed(4)), heapDeltaMB: Number(((process.memoryUsage().heapUsed - beforeHeap) / 1024 / 1024).toFixed(3)) }
}
