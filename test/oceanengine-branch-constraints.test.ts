import assert from 'node:assert/strict'
import test from 'node:test'
import {
  oceanEngineLeadCaptureMode,
  oceanEngineOptimizationTargetContext,
  optimizationCapabilitySelectionMatches,
} from '../src/lib/oceanengineBranchConstraints'
import type { PlatformConfiguration } from '../src/api/delivery'

type OceanProject = NonNullable<PlatformConfiguration['payload']['ocean_engine']>['project']

function project(overrides: Partial<OceanProject> = {}): OceanProject {
  return {
    draft_schema_version: 'oceanengine-configuration/v1', project_draft_id: 'project-1',
    account_reference: { namespace: 'oceanengine', object_kind: 'advertiser_account', scope: 'account:1', id: '1', state: 'resolved' },
    marketing_purpose: 'lead_generation', marketing_scenario: 'short_video_image_text', carrier: 'owned_landing_page',
    delivery_mode: 'manual', targeting: { smart_expansion: false },
    schedule: { start_at: '2026-09-03T00:00:00+08:00', end_at: '2026-09-04T23:59:59+08:00', timezone: 'Asia/Shanghai' },
    budget_and_bidding: { currency: 'CNY', daily_budget_minor: 100, bidding_strategy: 'stable_cost', charging_mode: 'CPC' },
    project_name: 'test', ...overrides,
  }
}

test('smart lead orange and IM uses the complete observed branch', () => {
  assert.deepEqual(oceanEngineOptimizationTargetContext(project({ lead_capture_mode: 'smart_lead', carrier: 'orange_landing_page_and_im' })), {
    campaign_type: 1, landing_type: 1, asset_type: 2, micro_app_id: '', cdp_marketing_goal: 1,
    dpa_ad_type: 0, micro_promotion_type: 2, micro_app_instance_id: '', multi_asset_types: [2, 1002], need_assets: false,
  })
})

test('sales lead carrier paths produce distinct capability contexts', () => {
  assert.deepEqual(oceanEngineOptimizationTargetContext(project({ lead_capture_mode: 'smart_lead', carrier: 'orange_landing_page' }))?.multi_asset_types, [2])
  assert.equal(oceanEngineOptimizationTargetContext(project({ lead_capture_mode: 'custom_lead', carrier: 'orange_landing_page' }))?.multi_asset_types, undefined)
  assert.deepEqual(oceanEngineOptimizationTargetContext(project({ lead_capture_mode: 'custom_lead', carrier: 'owned_landing_page' })), {
    campaign_type: 1, landing_type: 1, asset_type: 3, micro_app_id: '', cdp_marketing_goal: 1,
    dpa_ad_type: 0, micro_promotion_type: 2, micro_app_instance_id: '', need_assets: true,
  })
  assert.equal(oceanEngineOptimizationTargetContext(project({ lead_capture_mode: 'custom_lead', carrier: 'im' }))?.asset_type, 1002)
})

test('legacy sales-lead drafts infer a consistent lead capture mode from the carrier', () => {
  assert.equal(oceanEngineLeadCaptureMode(project({ lead_capture_mode: undefined, carrier: 'owned_landing_page' })), 'custom_lead')
  assert.equal(oceanEngineLeadCaptureMode(project({ lead_capture_mode: undefined, carrier: 'im' })), 'custom_lead')
  assert.equal(oceanEngineLeadCaptureMode(project({ lead_capture_mode: undefined, carrier: 'orange_landing_page' })), 'smart_lead')
  assert.equal(oceanEngineLeadCaptureMode(project({ lead_capture_mode: 'custom_lead', carrier: 'orange_landing_page' })), 'custom_lead')
})

test('non-lead paths do not use the lead capability endpoint', () => {
  assert.equal(oceanEngineOptimizationTargetContext(project({ marketing_purpose: 'ecommerce' })), undefined)
})

test('selection is valid only for the current immutable capability snapshot', () => {
  const selected = project({ optimization_target_reference: {
    namespace: 'oceanengine_capability', object_kind: 'optimization_target', scope: 'account:1', id: '2', state: 'resolved',
    audit_attributes: { capability_snapshot_id: 'snapshot-1' },
  } })
  assert.equal(optimizationCapabilitySelectionMatches(selected, 'snapshot-1', ['2', '100']), true)
  assert.equal(optimizationCapabilitySelectionMatches(selected, 'snapshot-2', ['2', '100']), false)
  assert.equal(optimizationCapabilitySelectionMatches(selected, 'snapshot-1', ['100']), false)
})
