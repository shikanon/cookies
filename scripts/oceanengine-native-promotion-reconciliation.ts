import type { OceanEngineFormPlan } from "./oceanengine-form-plan-compiler.ts";
import type { FieldReconciliation } from "./browser-rpa-runner-v3.ts";

export function isNativePromotionPlan(plan: OceanEngineFormPlan) {
  return plan.steps.some(step => step.field_key === "promotion.title_mode"
    || (step.field_key === "promotion.base_materials" && record(step.value)?.material_type === "douyin_video"));
}

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown> : undefined;
}

export function reconcileNativePromotionDetail(
  plan: OceanEngineFormPlan,
  promotionID: string,
  payload: unknown,
): FieldReconciliation {
  const material = record(plan.steps.find(step => step.field_key === "promotion.base_materials")?.value);
  const expectedVideo = material?.object_id;
  const expectedMode = plan.steps.find(step => step.field_key === "promotion.title_mode")?.value;
  const expectedName = plan.steps.find(step => step.field_key === "promotion.promotion_name")?.value;
  const fields: FieldReconciliation["fields"] = [
    { field_key: "promotion.base_materials", ...(typeof expectedVideo === "string" ? { expected: [expectedVideo] } : {}), status: "not_checked" },
    { field_key: "promotion.title_mode", ...(typeof expectedMode === "string" ? { expected: expectedMode } : {}), status: "not_checked" },
  ];
  const response = record(payload);
  const detail = record(record(response?.data)?.[promotionID]);
  if (response?.code !== 0 || !detail
    || detail.id !== promotionID || detail.advertiser_id !== plan.account_reference
    || !plan.parent_project_reference || detail.project_id !== plan.parent_project_reference
    || typeof expectedName !== "string" || detail.name !== expectedName
    || detail.is_del !== 0 || detail.project_is_del !== 0
    || material?.material_type !== "douyin_video" || typeof expectedVideo !== "string" || !/^\d+$/.test(expectedVideo)
    || expectedMode !== "投放原视频标题") {
    return { status: "not_checked", fields };
  }

  const videos = record(detail.material_group)?.video_material_info;
  if (Array.isArray(videos)) {
    const ids = videos.map(video => record(video)?.aweme_item_id);
    if (ids.every(id => typeof id === "string" && /^\d+$/.test(id))) {
      fields[0].observed = ids as string[];
      fields[0].status = ids.length === 1 && ids[0] === expectedVideo ? "matched" : "drifted";
    }
  }
  const titleSwitch = record(record(detail.data)?.native_info)?.origin_item_title_switch;
  if (titleSwitch === 1 || titleSwitch === 0) {
    fields[1].observed = titleSwitch === 1 ? "投放原视频标题" : "手动添加";
    fields[1].status = titleSwitch === 1 ? "matched" : "drifted";
  }
  return {
    status: fields.some(field => field.status === "not_checked") ? "not_checked"
      : fields.some(field => field.status === "drifted") ? "drifted" : "matched",
    fields,
  };
}
