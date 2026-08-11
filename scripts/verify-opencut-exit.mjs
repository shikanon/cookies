import { access, readFile } from 'node:fs/promises'
import { dirname, extname, resolve } from 'node:path'

const sourceRoot = resolve('src')
const entry = resolve(sourceRoot, 'main.tsx')
const forbidden = new Set([
  resolve(sourceRoot, 'features/video-editing/timeline.ts'),
  resolve(sourceRoot, 'features/video-editing/VideoTimeline.tsx'),
])
const visited = new Set()
const imports = /(?:from\s*|import\s*\()\s*['"](\.[^'"]+)['"]/g

await visit(entry)
const reachable = [...forbidden].filter(file => visited.has(file))
if (reachable.length) {
  throw new Error(`OpenCut exit boundary is still reachable from the production entry: ${reachable.join(', ')}`)
}

const notice = await readFile(resolve('third_party/opencut-timeline/NOTICE.md'), 'utf8')
const sbom = JSON.parse(await readFile(resolve('third_party/opencut-timeline/SBOM.spdx.json'), 'utf8'))
const pinnedSHA = 'cf5e79e919144200294fb9fed22a222592a0aeea'
if (!notice.includes(pinnedSHA) || !JSON.stringify(sbom).includes(pinnedSHA)) {
  throw new Error('OpenCut NOTICE and SBOM must identify the approved pinned commit')
}
process.stdout.write(`OpenCut exit drill passed: ${visited.size} production modules do not depend on the isolated adaptation.\n`)

async function visit(file) {
  if (visited.has(file)) return
  visited.add(file)
  const contents = await readFile(file, 'utf8')
  const runtimeContents = contents.replace(/^\s*import\s+type\s+.*$/gm, '')
  for (const match of runtimeContents.matchAll(imports)) {
    const target = await resolveImport(file, match[1])
    if (target?.startsWith(sourceRoot)) await visit(target)
  }
}

async function resolveImport(importer, specifier) {
  const base = resolve(dirname(importer), specifier)
  const candidates = extname(base)
    ? [base]
    : [base + '.ts', base + '.tsx', base + '.js', base + '.jsx', resolve(base, 'index.ts'), resolve(base, 'index.tsx')]
  for (const candidate of candidates) {
    try { await access(candidate); return candidate } catch { /* try the next supported module suffix */ }
  }
  return null
}
