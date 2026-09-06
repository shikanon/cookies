import { readFile } from "node:fs/promises";

import {
  compileEcommerceParentCondition,
  type EcommerceParentConditionManifest,
  type EcommerceParentContext,
} from "./oceanengine-ecommerce-field-compiler.ts";

export type OceanEnginePlanKind =
  | "project_create"
  | "project_edit"
  | "promotion_create"
  | "promotion_edit";

export type OceanEnginePlanCompilerInput = {
  schema_version: "oceanengine-form-plan-compiler-input/v1";
  account_reference: string;
  object_reference?: string;
  parent_project_reference?: string;
  parent_context: EcommerceParentContext;
  values: Record<string, unknown>;
};

export type OceanEngineExecutionAuthority = {
  schema_version: "browser-rpa-execution-authority/v1";
  authority_id: string;
  plan_sha256: string;
  confirm_token_sha256: string;
  issued_at: string;
  expires_at: string;
  account_reference: string;
  permitted_plan_kind: OceanEnginePlanKind;
  maximum_money_cny: number;
  schedule_date: string;
  maximum_final_clicks: 1;
};

type CompiledField = {
  key: string;
  operation: string;
  scope: string;
  target: string;
  required: boolean;
};

export type OceanEngineFormPlan = {
  schema_version: "oceanengine-playwright-rpa-plan/v3";
  plan_kind: OceanEnginePlanKind;
  browser: "msedge";
  mode: "prepare" | "submit";
  status: "ready" | "blocked";
  account_reference: string;
  object_reference?: string;
  parent_project_reference?: string;
  parent_condition_manifest_id: string;
  parent_context: EcommerceParentContext;
  blocked_reasons: string[];
  execution_authority?: OceanEngineExecutionAuthority;
  steps: Array<{
    id: string;
    kind: "identify_page" | "field_action" | "readback" | "final_click_boundary";
    page_kind: OceanEnginePlanKind;
    field_key?: string;
    operation?: string;
    scope?: string;
    target?: string;
    value?: unknown;
    value_state?: "provided" | "missing";
    required?: boolean;
    money_constraint?: {
      schema_version: "oceanengine-bid-constraints/v1";
      charging_mode: "CPC" | "CPM" | "OCPC" | "OCPM";
      minimum_minor: number;
      maximum_minor: number;
      maximum_source: "static" | "daily_budget";
    };
    remote_write: boolean;
    blocked: boolean;
    block_reason?: string;
  }>;
  allow_remote_write: boolean;
  maximum_final_clicks: 0 | 1;
};

function isProjectPlan(kind: OceanEnginePlanKind) {
  return kind === "project_create" || kind === "project_edit";
}

function requiredIdentityReason(kind: OceanEnginePlanKind, input: OceanEnginePlanCompilerInput) {
  if ((kind === "project_edit" || kind === "promotion_edit") && !input.object_reference) {
    return "missing_object_reference";
  }
  if ((kind === "promotion_create" || kind === "promotion_edit") && !input.parent_project_reference) {
    return "missing_parent_project_reference";
  }
  return null;
}

export function compileOceanEngineFormPlan(
  kind: OceanEnginePlanKind,
  manifest: EcommerceParentConditionManifest,
  input: OceanEnginePlanCompilerInput,
): OceanEngineFormPlan {
  if (input.schema_version !== "oceanengine-form-plan-compiler-input/v1") {
    throw new Error("unsupported form-plan compiler input");
  }
  if (!input.account_reference.trim()) throw new Error("account_reference is required");

  const parent = compileEcommerceParentCondition(manifest, input.parent_context);
  const fields: CompiledField[] = isProjectPlan(kind) ? parent.project.fields : parent.promotion.fields;
  const blockedReasons = [...parent.blocked_reasons];
  const identityReason = requiredIdentityReason(kind, input);
  if (identityReason) blockedReasons.push(identityReason);

  const fieldSteps = fields.map((field, index) => {
    const hasValue = Object.hasOwn(input.values, field.key);
    if (field.required && !hasValue) blockedReasons.push(`missing_required_value:${field.key}`);
    const target = kind === "project_create" && field.key === "project.marketing_product_reference"
      ? "点击选择商品"
      : kind === "project_edit" && field.key === "project.marketing_product_reference"
        ? "更换"
        : field.target;
    return {
      id: `${String(index + 2).padStart(3, "0")}-${field.key}`,
      kind: "field_action" as const,
      page_kind: kind,
      field_key: field.key,
      operation: field.operation,
      scope: field.scope,
      target,
      ...(hasValue ? { value: input.values[field.key] } : {}),
      value_state: hasValue ? "provided" as const : "missing" as const,
      required: field.required,
      remote_write: false,
      blocked: false,
    };
  });

  const uniqueBlockedReasons = [...new Set(blockedReasons)];
  return {
    schema_version: "oceanengine-playwright-rpa-plan/v3",
    plan_kind: kind,
    browser: "msedge",
    mode: "prepare",
    status: uniqueBlockedReasons.length === 0 ? "ready" : "blocked",
    account_reference: input.account_reference,
    ...(input.object_reference ? { object_reference: input.object_reference } : {}),
    ...(input.parent_project_reference ? { parent_project_reference: input.parent_project_reference } : {}),
    parent_condition_manifest_id: parent.manifest_id,
    parent_context: input.parent_context,
    blocked_reasons: uniqueBlockedReasons,
    steps: [
      {
        id: "001-identify-page",
        kind: "identify_page",
        page_kind: kind,
        remote_write: false,
        blocked: false,
      },
      ...fieldSteps,
      {
        id: `${String(fieldSteps.length + 2).padStart(3, "0")}-readback`,
        kind: "readback",
        page_kind: kind,
        remote_write: false,
        blocked: false,
      },
      {
        id: `${String(fieldSteps.length + 3).padStart(3, "0")}-final-click-boundary`,
        kind: "final_click_boundary",
        page_kind: kind,
        target: isProjectPlan(kind) ? "保存并新建单元" : "保存并关闭",
        remote_write: true,
        blocked: true,
        block_reason: "PREPARE_PLAN_REMOTE_WRITE_PROHIBITED",
      },
    ],
    allow_remote_write: false,
    maximum_final_clicks: 0,
  };
}

export async function runFormPlanCompilerCli(kind: OceanEnginePlanKind) {
  const [manifestPath, inputPath] = process.argv.slice(2);
  if (!manifestPath || !inputPath) {
    throw new Error(`Usage: tsx scripts/oceanengine-${kind.replaceAll("_", "-")}-plan.ts MANIFEST.json INPUT.json`);
  }
  const manifest = JSON.parse(await readFile(manifestPath, "utf8")) as EcommerceParentConditionManifest;
  const input = JSON.parse(await readFile(inputPath, "utf8")) as OceanEnginePlanCompilerInput;
  process.stdout.write(`${JSON.stringify(compileOceanEngineFormPlan(kind, manifest, input), null, 2)}\n`);
}
