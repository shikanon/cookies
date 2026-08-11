# Upstream policy

OpenCut Classic commit `cf5e79e919144200294fb9fed22a222592a0aeea` is the only approved source snapshot for the first editor phase. It is archived upstream and is not a runtime or package dependency.

Any additional copied or adapted source must update `NOTICE.md`, pass timeline unit and end-to-end tests, and receive a license/dependency review. The floating OpenCut `main` branch must not be merged automatically.

The removal boundary is `src/features/video-editing/timeline.ts` plus
`src/features/video-editing/VideoTimeline.tsx`. They are no longer reachable
from the production entry, and `npm run test:video-editor-governance` enforces
that boundary. Cookies API and database records use cookies-owned v1/v2
schemas, so deleting the isolated adaptation requires no persisted-data
migration. Its legacy behavior tests may be removed or rewritten separately
without changing the C7 editor.

Upgrade review must compare the pinned source, update `NOTICE.md` and
`SBOM.spdx.json`, rerun the timeline benchmark and interaction E2E, and receive
an explicit maintainer approval. Floating upstream changes are never merged
automatically.
