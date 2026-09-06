# Ocean Engine replay fixtures

These fixtures are synthetic, redacted payloads for deterministic adapter and normalization tests. They contain no Cookie, CSRF token, advertiser ID, project name, promotion name, signed URL, or unredacted platform response.

`web-api-contract-v1.json` records the observed Secsdk and endpoint contract.
Its `request_captured_response_pending` state keeps production writes blocked.
The captured browser writes included `_signature` and `x-sessionid`. Their
presence does not prove that the server requires them. Existing Connector reads
omit both fields. Test omission before adding either field to the write Client.
`web-api-request-shapes-v1.json` stores names and types only. The raw HAR and
all scalar request values stay outside Git.

The first direct-HTTP probe stopped at the Secsdk HEAD request. HTTP status was
200, but the token response header was absent. No project POST was sent. The
fixture records only this non-secret result and the zero-match reconciliation.

The declared Edge page produced the same HEAD result. Secsdk then returned its
official `DOWNGRADE` fallback. Both captured browser writes used that exact
fallback value. No token value is stored in the fixture.

One approved direct-HTTP probe used that fallback. It sent one POST without
`_signature` or `x-sessionid`. The response had HTTP 200 and business code
`50100`. Reconciliation found no project. This is a deterministic rejection.

The public bundle source map identifies `_signature` as output from
`@byted/acrawler@1.6.8`. The input contains the exact URL and normalized body.
The output also contains module-load clock state. No test found account,
Cookie, DOM, or device input. Do not reuse a captured signature.

The create-page bundle initializes `window.sessionId` with a random UUID v4.
Captured project and promotion requests used different UUID values. The
fixture records only their value class. It does not store either value.

One approved session-ID isolation POST still returned business code `50100`.
The first signature POST signed after `aadvid` was present. The browser signs
before it adds `aadvid`, so that experiment is invalid. The corrected probe
generates a 31-character `_signature` without a platform write. It verifies the
published acrawler runtime with a fixed SHA-256 first. The fixture does not
store the signature. Every `50100` rejection above shared one payload defect:
the probe replaced the captured epoch schedule strings with literal dates.
After the probe adopted the epoch contract, one approved POST returned code
`0` with a platform project ID, and the operator confirmed the object by its
exact name digest. This is the first confirmed Web API create.

Fixture rules:

- Platform identifiers are intentionally opaque strings, including values larger than JavaScript's safe integer range.
- Empty reports prove schema-only synchronization and must not become all-zero metric facts.
- `quality` is explicit; unknown mappings are represented as `mapping_incomplete`.
- Fixtures are not evidence from a real account and must never be presented as live platform data.
