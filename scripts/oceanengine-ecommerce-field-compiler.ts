import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

export type EcommerceParentContext = {
  carrier: string;
  optimization_target: string;
  optimization_target_external_action?: string;
  deep_optimization: string;
  delivery_mode: string;
  placement_mode: string;
  search_targeting_expansion?: boolean;
  parent_references?: Record<string, string>;
};

type FieldControl = {
  operation: string;
  scope: string;
  target: string;
  required: boolean;
};

type Rule = {
  add_fields?: string[];
  remove_fields?: string[];
  project_add_fields?: string[];
  project_remove_fields?: string[];
  required_parent_reference?: string;
  required_optimization_target?: string;
  required_delivery_modes?: string[];
  required_delivery_mode?: string;
  landing_page_target?: string;
  billing?: string;
  search_boost?: "enabled" | "disabled";
  project_roi_coefficient?: boolean;
  available_media?: string[];
  material_limits?: Record<string, number>;
};

export type EcommerceParentConditionManifest = {
  schema_version: "oceanengine-ecommerce-parent-condition-manifest/v1";
  manifest_id: string;
  evidence_ref: string;
  remote_write_authorized: false;
  dimensions: Record<string, string[]>;
  project_field_definitions: Record<string, FieldControl>;
  base_project_fields: string[];
  field_definitions: Record<string, FieldControl>;
  base_promotion_fields: string[];
  carrier_rules: Record<string, Rule>;
  delivery_rules: Record<string, Rule>;
  optimization_rules: Record<string, Rule>;
  deep_optimization_rules: Record<string, Rule>;
  placement_rules: Record<string, Rule>;
  search_targeting_expansion_rule: { requires_search_boost: "enabled" };
};

export type CompiledEcommerceFields = {
  status: "ready" | "blocked";
  manifest_id: string;
  context: EcommerceParentContext;
  blocked_reasons: string[];
  project: {
    billing: string | null;
    search_boost: "enabled" | "disabled";
    search_targeting_expansion: boolean;
    roi_coefficient: boolean;
    available_media: string[];
    fields: Array<FieldControl & { key: string }>;
  };
  promotion: {
    material_limits: Record<string, number>;
    fields: Array<FieldControl & { key: string }>;
  };
  remote_write_authorized: false;
};

function requireKnownValue(manifest: EcommerceParentConditionManifest, dimension: string, value: string) {
  if (!manifest.dimensions[dimension]?.includes(value)) {
    throw new Error(`unknown ${dimension}: ${value}`);
  }
}

function applyFieldDelta(keys: string[], rule: Rule) {
  const removed = new Set(rule.remove_fields ?? []);
  const result = keys.filter((key) => !removed.has(key));
  for (const key of rule.add_fields ?? []) {
    if (!result.includes(key)) result.push(key);
  }
  return result;
}

function applyProjectFieldDelta(keys: string[], rule: Rule) {
  return applyFieldDelta(keys, {
    add_fields: rule.project_add_fields,
    remove_fields: rule.project_remove_fields,
  });
}

export function compileEcommerceParentCondition(
  manifest: EcommerceParentConditionManifest,
  context: EcommerceParentContext,
): CompiledEcommerceFields {
  if (manifest.schema_version !== "oceanengine-ecommerce-parent-condition-manifest/v1") {
    throw new Error("unsupported parent-condition manifest");
  }
  if (manifest.remote_write_authorized !== false) {
    throw new Error("a field manifest cannot authorize remote writes");
  }

  for (const dimension of ["carrier", "optimization_target", "deep_optimization", "delivery_mode", "placement_mode"]) {
    requireKnownValue(manifest, dimension, context[dimension as keyof EcommerceParentContext] as string);
  }

  const carrier = manifest.carrier_rules[context.carrier];
  const delivery = manifest.delivery_rules[context.delivery_mode];
  const optimization = manifest.optimization_rules[context.optimization_target];
  const deep = manifest.deep_optimization_rules[context.deep_optimization];
  const placement = manifest.placement_rules[context.placement_mode];
  const blockedReasons: string[] = [];

  if (carrier.required_parent_reference && !context.parent_references?.[carrier.required_parent_reference]) {
    blockedReasons.push(`missing_parent_reference:${carrier.required_parent_reference}`);
  }
  if (deep.required_optimization_target && deep.required_optimization_target !== context.optimization_target) {
    blockedReasons.push(`deep_optimization_requires_target:${deep.required_optimization_target}`);
  }
  if (deep.required_delivery_modes && !deep.required_delivery_modes.includes(context.delivery_mode)) {
    blockedReasons.push(`deep_optimization_unsupported_delivery_mode:${context.delivery_mode}`);
  }
  if (placement.required_delivery_mode && placement.required_delivery_mode !== context.delivery_mode) {
    blockedReasons.push(`placement_requires_delivery_mode:${placement.required_delivery_mode}`);
  }

  const searchBoost = deep.search_boost ?? optimization.search_boost ?? "disabled";
  if (context.search_targeting_expansion && searchBoost !== manifest.search_targeting_expansion_rule.requires_search_boost) {
    blockedReasons.push("search_targeting_expansion_unavailable");
  }

  let fieldKeys = [...manifest.base_promotion_fields];
  for (const rule of [delivery, optimization, deep, placement]) fieldKeys = applyFieldDelta(fieldKeys, rule);

  let projectFieldKeys = [...manifest.base_project_fields];
  for (const rule of [delivery, optimization, deep, placement]) projectFieldKeys = applyProjectFieldDelta(projectFieldKeys, rule);
  if (searchBoost === "disabled") {
    projectFieldKeys = projectFieldKeys.filter(
      (key) => key !== "project.search_bid_coefficient" && key !== "project.search_targeting_expansion",
    );
  }

  const fields = fieldKeys.map((key) => {
    const definition = manifest.field_definitions[key];
    if (!definition) throw new Error(`missing field definition: ${key}`);
    if (key === "promotion.landing_page_reference" && carrier.landing_page_target) {
      return { key, ...definition, target: carrier.landing_page_target };
    }
    return { key, ...definition };
  });
  const projectFields = projectFieldKeys.map((key) => {
    const definition = manifest.project_field_definitions[key];
    if (!definition) throw new Error(`missing project field definition: ${key}`);
    if (key === "project.carrier") {
      const carrierTargets: Record<string, string> = {
        orange_landing_page: "橙子落地页",
        owned_landing_page: "自研落地页",
        byte_miniapp: "字节小程序",
        wechat_miniapp: "微信小程序",
      };
      return { key, ...definition, target: carrierTargets[context.carrier] };
    }
    if (key === "project.deep_optimization_mode") {
      const deepTargets: Record<string, string> = {
        disabled: "不启用",
        conversion_roi: "成交ROI",
        net_order: "净成交下单",
        net_roi: "净成交ROI",
      };
      return { key, ...definition, target: deepTargets[context.deep_optimization] };
    }
    if (key === "project.delivery_mode") {
      return {
        key,
        ...definition,
        target: context.delivery_mode === "manual" ? "手动投放" : "自动投放(UBMax)",
      };
    }
    if (key === "project.placement_strategy") {
      return {
        key,
        ...definition,
        target: context.placement_mode === "preferred_media" ? "首选媒体" : "通投智选",
      };
    }
    return { key, ...definition };
  });

  return {
    status: blockedReasons.length === 0 ? "ready" : "blocked",
    manifest_id: manifest.manifest_id,
    context,
    blocked_reasons: blockedReasons,
    project: {
      billing: optimization.billing ?? null,
      search_boost: searchBoost,
      search_targeting_expansion: Boolean(context.search_targeting_expansion),
      roi_coefficient: Boolean(deep.project_roi_coefficient),
      available_media: [...(placement.available_media ?? [])],
      fields: projectFields,
    },
    promotion: {
      material_limits: { ...(delivery.material_limits ?? {}) },
      fields,
    },
    remote_write_authorized: false,
  };
}

async function main() {
  const [manifestPath, contextPath] = process.argv.slice(2);
  if (!manifestPath || !contextPath) {
    throw new Error("Usage: tsx scripts/oceanengine-ecommerce-field-compiler.ts MANIFEST.json CONTEXT.json");
  }
  const manifest = JSON.parse(await readFile(manifestPath, "utf8")) as EcommerceParentConditionManifest;
  const context = JSON.parse(await readFile(contextPath, "utf8")) as EcommerceParentContext;
  process.stdout.write(`${JSON.stringify(compileEcommerceParentCondition(manifest, context), null, 2)}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) void main();
