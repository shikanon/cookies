import assert from "node:assert/strict";
import test from "node:test";
import { reconcileProjectDetail } from "../scripts/oceanengine-project-reconciliation.ts";
import type { OceanEngineFormPlan } from "../scripts/oceanengine-form-plan-compiler.ts";

const plan = { account_reference: "123", parent_context: { optimization_target_external_action: "20" }, steps: [{ field_key: "project.project_name", value: "test-project" }] } as OceanEngineFormPlan;
const row = { id: "456", name: "test-project", advertiser_id: "123", is_del: 0, data: { promotion_strategy: { external_action: 20 } } };
test("project recovery compares the persisted target and exact identity", () => {
  const result = (change: Record<string, unknown>) => reconcileProjectDetail(plan, "456", { code: 0, data: { "456": { ...row, ...change } } });
  assert.equal(result({}).status, "matched");
  assert.equal(result({ data: { promotion_strategy: { external_action: 2 } } }).status, "drifted");
  for (const change of [{ advertiser_id: "789" }, { id: "789" }, { name: "other" }, { data: {} }, { is_del: 1 }]) assert.equal(result(change).status, "not_checked");
  assert.equal(reconcileProjectDetail(plan, "456", { code: 0, data: { "789": row } }).status, "not_checked");
  assert.equal(reconcileProjectDetail(plan, "456", undefined).status, "not_checked");
});
