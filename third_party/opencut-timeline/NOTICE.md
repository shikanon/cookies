# OpenCut Timeline Notice

The cookies video-editing timeline command model is adapted from OpenCut Classic.

- Upstream: https://github.com/OpenCut-app/opencut-classic
- Fixed commit: `cf5e79e919144200294fb9fed22a222592a0aeea`
- License: MIT, reproduced in `LICENSE`
- Copyright: 2025-2026 OpenCut

Reviewed upstream sources:

- `apps/web/src/commands/timeline/element/move-elements.ts`
- `apps/web/src/commands/timeline/element/split-elements.ts`
- `apps/web/src/core/managers/commands.ts`
- `apps/web/src/timeline/controllers/resize-controller.ts`
- `apps/web/src/timeline/controllers/zoom-controller.ts`
- `apps/web/src/timeline/snapping/`

The retained adaptation is isolated in:

- `src/features/video-editing/timeline.ts` for command and snapping calculations;
- `src/features/video-editing/VideoTimeline.tsx` for playhead, resize, zoom,
  drag/drop, and visible snapping interactions.

The production C7 editor uses cookies-owned `visualTimeline.ts`,
`VideoEditingCanvasWorkspace.tsx`, AssetVersionRef, Go APIs, and the FFmpeg
render pipeline. It does not import either isolated adaptation file. The
automated exit drill in `scripts/verify-opencut-exit.mjs` walks runtime imports
from `src/main.tsx` and fails if the isolated boundary becomes reachable.
No OpenCut runtime package is installed.
