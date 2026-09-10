import type { OceanEngineFormPlan } from "./oceanengine-form-plan-compiler.ts";
import type { FieldReconciliation } from "./browser-rpa-runner-v3.ts";

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown> : undefined;
}

export function reconcileProjectDetail(plan: OceanEngineFormPlan, projectID: string, payload: unknown): FieldReconciliation {
  const expected = plan.parent_context.optimization_target_external_action;
  const fields: FieldReconciliation["fields"] = [{ field_key: "project.optimization_target_reference", expected, status: "not_checked" }];
  const response = record(payload);
  const expectedName = plan.steps.find(step => step.field_key === "project.project_name")?.value;
  if (response?.code !== 0 || !expected || !/^\d+$/.test(projectID)) return { status: "not_checked", fields };
  const row = record(record(response.data)?.[projectID]);
  if (!row || row.id !== projectID || row.name !== expectedName || row.advertiser_id !== plan.account_reference || row.is_del !== 0) return { status: "not_checked", fields };
  const value = record(record(row.data)?.promotion_strategy)?.external_action;
  if (typeof value !== "string" && typeof value !== "number") return { status: "not_checked", fields };
  fields[0].observed = String(value);
  fields[0].status = String(value) === expected ? "matched" : "drifted";
  return { status: fields[0].status, fields };
}
