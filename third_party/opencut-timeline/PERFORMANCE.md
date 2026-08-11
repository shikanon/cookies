# Video timeline performance and OpenCut exit baseline

## Baseline identity

- Date: 2026-08-11
- OpenCut Classic source: `cf5e79e919144200294fb9fed22a222592a0aeea`
- Runtime: Node.js v24, Windows development workstation
- Commands: `node scripts/verify-opencut-exit.mjs` and `npx tsx scripts/benchmark-video-timeline.ts`

## Exit drill

The production import graph contains 114 modules and does not reach the two
isolated OpenCut-adapted files. CI repeats this assertion. The persisted v1/v2
timeline contracts and Go renderer do not depend on OpenCut.

## Editor Kernel benchmark

Each fixture distributes visual clips across three tracks and performs 2,000
immutable moves. These are repeatable command-kernel measurements, not claims
about browser frame rate.

| Clips | p50 | p95 | maximum |
| ---: | ---: | ---: | ---: |
| 10 | 0.0015 ms | 0.0031 ms | 0.2474 ms |
| 50 | 0.0037 ms | 0.0054 ms | 6.5974 ms |
| 100 | 0.0051 ms | 0.0063 ms | 0.2058 ms |

The executable gate fails when p95 exceeds 5 ms, one command exceeds 50 ms,
or heap growth exceeds 64 MiB. Browser acceptance separately checks 1280,
1366, 1440, and 1920 pixel desktop widths. Any future upstream review must
rerun both gates and explain an approved regression before changing the pin.
