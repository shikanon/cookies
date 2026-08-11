import { copyFile, mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'

const root = resolve('.data/e2e-platform-blobs/cookies-assets/demo/investor')
const fixtures = resolve('.tmp/video-editor-e2e')

await mkdir(root, { recursive: true })
await Promise.all([
  copyFile(resolve(fixtures, 'video-original-audio.mp4'), resolve(root, 'creative-video.mp4')),
  copyFile(resolve(fixtures, 'overlay-opaque.png'), resolve(root, 'creative-image.png')),
])
