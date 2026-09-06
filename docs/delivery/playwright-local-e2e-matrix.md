# Playwright local end-to-end matrix

## Scope

This matrix covers the local Playwright Edge v3 route for PR #68. It excludes
remote browser deployment. Every live write uses a disposable project, a future
start date, and the current page minimum budget and bid.

## Automated chain

| Layer | Covered behavior | Status |
| --- | --- | --- |
| Cookies UI | Select Web API or Playwright before execution | covered |
| Delivery API | Send and validate `execution_driver` | covered |
| ChangeSet | Freeze the driver in authority JSON and canonical hash | covered |
| Controlled target | Use a driver-specific object fingerprint | covered |
| Browser RPA Run | Persist the same driver and reject authority mismatch | covered |
| Worker | Dispatch only to the adapter selected by the Run | covered |
| Safe retry | Keep the failed Run's original driver | covered |
| Runner v3 | Prepare stops before the final click | covered |
| Runner v3 | Submit consumes one confirmation and permits one click | covered |
| Staged create | Create one project, then one or more promotions | covered |
| Result safety | Preserve confirmed IDs and stop on drift or unknown result | covered |

## Local live-path target

| Priority | Project path | Promotion path | Expected evidence |
| --- | --- | --- | --- |
| P0 | Ecommerce, short-video/image-text, manual delivery, Orange landing page | Account identity, one video, product image, CTA set | project and promotion IDs, field readback, list readback |
| P0 | Ecommerce, UBMax, owned landing page | Manual landing URL and manual direct link | exact URL readback and no object-binding false block |
| P0 | Lead generation, UBMax, owned landing page | Custom lead mode | parent-condition fields and promotion create |
| P0 | Ecommerce project with multiple promotions | Two or more ordered promotion creates | a fresh confirmation and ID write-back for each object |
| P0 | Existing confirmed project | Create one additional promotion | parent project ID remains fixed |
| P1 | Confirmed promotion | Budget edit | before/after value and list readback |
| P1 | Confirmed project and promotion | Edit-form Prepare only | complete diff with no final click |

## Stable non-success paths

These paths must stop with a clear reason. They do not count as live success:

- Byte mini-app and WeChat mini-app carriers.
- An unavailable application event asset.
- A platform branch that the current account does not expose.
- An unbound material, product, landing page, category, or brand.
- More than one base material or product image in one Runner v3 form.
- Product images without a stable source identity.
- Landing-page or CTA reconciliation that remains `not_checked`.

## Live run on 2026-09-01

The local Edge session passed CDP, login, and account checks. The disposable
project `Cookies-PW-E2E-0902-01` was created with platform ID
`7680450191750053907`. Cookies saved the project mapping.

The first promotion Prepare exposed two defects. The image picker renders the
selected product image in both the selected card and the submit-bar preview.
Runner v3 now verifies the stable image source only inside the selected card.
The staged safe-retry path now creates a new Run when a later Prepare fails
after an earlier project stage succeeded. It still blocks retries after an
unknown submit result.

The retried promotion Prepare completed all 21 steps. It read one video, one
product image, the landing page, the CTA set, CNY 300 daily budget, and CNY
0.01 bid. The final-click boundary remained closed during Prepare.

The promotion Submit ended as `result_unknown`. A separate read-only list
query found no target promotion. Browser resource history also contained no
`create_promotion` request. The current control plane has no no-effect
reconciliation port, so no retry was performed. Remaining P0 live rows stay
blocked until the control plane can record this read-only no-effect result and
Runner v3 can report client-side submit rejection before it becomes an unknown
result.

## Completion rule

Most-path coverage requires all P0 rows to pass through this chain:

```text
Cookies UI
-> Delivery API
-> ChangeSet and approval
-> Browser RPA API
-> Worker
-> Playwright Runner v3
-> local Edge
-> OceanEngine
-> platform ID write-back
-> Cookies execution and entity pages
```

P1 rows can remain follow-up work. Remote Edge deployment is outside PR #68.
