import { expect, type APIRequestContext } from '@playwright/test'
import { execFileSync } from 'node:child_process'

export const runtimeAccountId = (projectId: string) => `oeacct_e2e_${projectId}`

export function seedRuntimeAccount(projectId: string) {
  const database = process.env.COOKIES_E2E_MYSQL_DATABASE ?? `cookies_e2e_${process.env.COOKIES_E2E_API_PORT ?? '18080'}`
  if (process.env.COOKIES_E2E_MYSQL_DSN || !/^cookies_e2e_[A-Za-z0-9_]+$/.test(database) || !/^[A-Za-z0-9_]+$/.test(projectId)) {
    throw new Error('Delivery account fixtures require the isolated local cookies_e2e database')
  }
  const account = runtimeAccountId(projectId)
  execFileSync('docker', ['compose', 'exec', '-T', 'mysql', 'mysql', '-uroot', '-proot_local_development_only', database], {
    input: `INSERT IGNORE INTO platform_accounts (id, organization_id, platform, external_id, display_label, status, verified_at, created_at, updated_at)
      VALUES ('${account}', 'org_local', 'ocean_engine', '${account}', 'E2E account fixture', 'verified', NOW(), NOW(), NOW());
      INSERT IGNORE INTO platform_account_connections (id, organization_id, project_id, account_id, connection_type, credential_ref, status, last_verified_at, created_at, updated_at)
      VALUES ('e2e_connection_${projectId}', 'org_local', '${projectId}', '${account}', 'web_api', '', 'verified', NOW(), NOW(), NOW());`,
    stdio: ['pipe', 'pipe', 'pipe'],
  })
}

export function deliveryRuntimePayload(projectId: string, suffix: string, version = 1, promotions = 1, name = `Delivery ${suffix}`) {
  const calibration_manifest = { schema_version: 'oceanengine-calibration-manifest/v1', manifest_id: 'oceanengine-calibration-current-test-account-2026-08-16' }
  const ref = (kind: string, id: string) => ({ namespace: 'cookies', object_kind: kind, scope: `project:${projectId}`, id, version: 'v1', content_hash: 'a'.repeat(64), state: 'resolved' })
  const material = ref('asset_version', 'asset_demo_investor_creative_video')
  return {
    expected_version: version - 1,
    intent: {
      schema_version: 'delivery-intent/v1', intent_id: `intent-${suffix}`, version_number: version, hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
      payload: {
        payload_schema_version: 'delivery-intent/v1', marketing_objective: 'qualified conversions', calibration_manifest,
        budget_boundary: { currency: 'CNY', minimum_total_minor: 0, maximum_total_minor: 300000 },
        schedule_boundary: { earliest_start: '2026-08-11T00:00:00+08:00', latest_end: '2026-08-25T00:00:00+08:00', timezone: 'Asia/Shanghai' },
        optimization_preferences: [], material_references: [material], audience_constraints: {}, strategy_reference: ref('strategy_version', 'task_demo_precision_strategy'),
      },
      configuration_provenance: { kind: 'manual', generator_ref: 'e2e' }, fact_provenance: { source: 'mock', snapshot_ref: `mock://${suffix}/intent/${version}` },
    },
    platform_configuration: {
      schema_version: 'delivery-platform-configuration/v2', configuration_id: `configuration-${suffix}`, version_number: version,
      platform: 'ocean_engine', profile_version: 'oceanengine-configuration/v1', hash_algorithm: 'RFC8785-JCS-SHA256(canonical_payload)',
      payload: { profile: 'ocean_engine', ocean_engine: { profile: 'ocean_engine', calibration_manifest, project: {
        draft_schema_version: 'oceanengine-configuration/v1', project_draft_id: `project-${suffix}-${version}`, account_reference: ref('advertiser_account', runtimeAccountId(projectId)),
        marketing_purpose: 'ecommerce', marketing_scenario: 'manual_delivery', carrier: 'landing_page', delivery_mode: 'manual', targeting: { smart_expansion: false },
        schedule: { start_at: '2026-08-11T00:00:00+08:00', end_at: '2026-08-25T00:00:00+08:00', timezone: 'Asia/Shanghai' },
        budget_and_bidding: { currency: 'CNY', daily_budget_minor: 20000, bidding_strategy: 'manual_bid', charging_mode: 'CPC', bid_minor: 100 }, project_name: name,
      }, promotions: Array.from({ length: promotions }, (_, index) => ({
        draft_schema_version: 'oceanengine-configuration/v1', promotion_draft_id: `promotion-${suffix}-${index + 1}`,
        delivery_identity: { mode: 'account_info' }, base_material_references: [material], copy_items: [{ text: `copy ${index + 1}` }], settings: {}, promotion_name: `Promotion ${index + 1}`,
      })) } },
      configuration_provenance: { kind: 'manual', generator_ref: 'e2e' }, fact_provenance: { source: 'mock', snapshot_ref: `mock://${suffix}/configuration/${version}` },
      compilation_metadata: { field_evidence: [{ field: 'project', state: 'operator_reviewed' }], evidence_refs: [] },
    },
  }
}

export async function createRuntimePlan(request: APIRequestContext, projectId: string, suffix: string) {
  seedRuntimeAccount(projectId)
  const payload = deliveryRuntimePayload(projectId, suffix)
  const response = await request.post(`/api/delivery/v1/projects/${projectId}/plans`, { data: { intent: payload.intent, platform_configuration: payload.platform_configuration } })
  expect(response.status(), await response.text()).toBe(201)
  return response.json() as Promise<any>
}
