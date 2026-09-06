// Fake RPA runner used by runner_test.go. The "CDP endpoint" argument doubles
// as the behavior selector. The plan arrives on stdin exactly like the real
// runner; the response is a single JSON result document on stdout.
import { createInterface } from 'node:readline'

const mode = process.argv[2] ?? 'success'
let data = ''
const stdin = createInterface({ input: process.stdin })
stdin.on('line', line => { data += line })
stdin.on('close', () => {
  const plan = JSON.parse(data)
  if (mode === 'garbage') {
    process.stdout.write('this is not json')
    return
  }
  if (mode === 'wrong-schema') {
    process.stdout.write(JSON.stringify({ schema_version: 'bogus/v0', outcome: 'success', error_code: 'ok', final_click_performed: false, steps: [] }))
    return
  }
  if (mode === 'page-drift') {
    process.stdout.write(JSON.stringify({
      schema_version: 'oceanengine-playwright-rpa-result/v1',
      outcome: 'failed',
      error_code: 'page_drift',
      error_message: 'scope locator missing',
      final_click_performed: false,
      steps: [],
    }))
    return
  }
  if (mode === 'noisy') {
    process.stdout.write('warning: third-party library noise on stdout\n')
  }
  if (plan.schema_version === 'oceanengine-playwright-rpa-plan/v3') {
    const notChecked = mode === 'v3-not-checked'
    if (mode === 'v3-no-effect') {
      process.stdout.write(JSON.stringify({
        schema_version: 'oceanengine-playwright-rpa-result/v2',
        outcome: 'failed',
        error_code: 'submit_no_effect_confirmed',
        final_click_performed: true,
        reconciliation: 'not_found',
        steps: [{ id: plan.steps?.[0]?.id ?? 'step', status: 'failed', readback: { reconciliation: 'not_found', platform_write_request_observed: false } }],
      }))
      return
    }
    process.stdout.write(JSON.stringify({
      schema_version: 'oceanengine-playwright-rpa-result/v2',
      outcome: 'success',
      error_code: 'ok',
      final_click_performed: plan.mode === 'submit',
      created_object_id: 'promotion_v3_test',
      reconciliation: 'matched',
      field_reconciliation: {
        status: notChecked ? 'not_checked' : 'matched',
        fields: [{ field_key: 'promotion.landing_page_reference', expected: 'landing_test', ...(notChecked ? {} : { observed: 'landing_test' }), status: notChecked ? 'not_checked' : 'matched' }],
      },
      steps: [{ id: plan.steps?.[0]?.id ?? 'step', status: 'succeeded', readback: { runner_args: process.argv.slice(3) } }],
    }))
    return
  }
  process.stdout.write(JSON.stringify({
    schema_version: 'oceanengine-playwright-rpa-result/v1',
    outcome: 'success',
    error_code: 'ok',
    final_click_performed: false,
    steps: [{
      id: plan.steps?.[0]?.id ?? 'step',
      status: 'succeeded',
      before_facts: { page_kind: plan.steps?.[0]?.page_kind ?? '' },
      readback: { object_id: 'promotion_test', plan_mode: plan.mode ?? '' },
      diff_keys: [],
      page_reference: 'https://ad.oceanengine.com/promotion/list',
    }],
  }))
})
