import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolve } from "node:path";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";

import {
  canonicalImageSourceIdentity,
  isStablePlatformImageSourceIdentity,
  executePlan,
  executePreparePlan,
  parseOceanEngineMoneyConstraint,
  PlaywrightPageOperations,
  type PageOperations,
  type ReconciliationResult,
  type SubmitObservation,
} from "../scripts/browser-rpa-runner-v3.ts";
import { reconcileNativePromotionDetail } from "../scripts/oceanengine-native-promotion-reconciliation.ts";
import type { Page } from "@playwright/test";
import { authorizeSubmitPlan } from "../scripts/rpa-authority.ts";
import { type EcommerceParentConditionManifest } from "../scripts/oceanengine-ecommerce-field-compiler.ts";
import {
  compileOceanEngineFormPlan,
  type OceanEngineFormPlan,
  type OceanEnginePlanCompilerInput,
} from "../scripts/oceanengine-form-plan-compiler.ts";

const root = resolve(import.meta.dirname, "..");
const readJSON = (relative: string) => JSON.parse(readFileSync(resolve(root, relative), "utf8"));
const manifest = readJSON("docs/delivery/fixtures/oceanengine-ecommerce-parent-condition-manifest-v1.json") as EcommerceParentConditionManifest;
const resultSchema = readJSON("docs/delivery/schemas/oceanengine-playwright-rpa-result-v2.json");
const validateResult = new Ajv2020({ allErrors: true, strict: false }).compile(resultSchema);
const planSetAjv = new Ajv2020({ allErrors: true, strict: false });
planSetAjv.addSchema(readJSON("docs/delivery/schemas/oceanengine-execution-authority-v1.json"));
planSetAjv.addSchema(readJSON("docs/delivery/schemas/oceanengine-playwright-rpa-plan-v3.json"));
const validatePlanSet = planSetAjv.compile(readJSON("docs/delivery/schemas/oceanengine-configuration-plan-set-v1.json"));
const runnerSource = readFileSync(resolve(root, "scripts/browser-rpa-runner-v3.ts"), "utf8");

test("project form navigation waits for the asynchronous New Project action", () => {
  assert.match(runnerSource, /attempt < 120 && !createProject/);
  assert.match(runnerSource, /the New Project action did not load/);
  assert.doesNotMatch(runnerSource, /the New Project action is unavailable/);
});

class FakePage implements PageOperations {
  readonly applied: string[] = [];
  identified = 0;
  finalClicks = 0;
  submitObservation: SubmitObservation = { outcome: "success" };
  reconciliation: ReconciliationResult = { status: "matched", created_object_id: "7677604041052405801" };
  moneyConstraint: { minimum_minor: number; maximum_minor: number } | undefined;

  async identifyPage() {
    this.identified += 1;
  }

  async applyField(step: OceanEngineFormPlan["steps"][number]) {
    if (step.field_key) this.applied.push(step.field_key);
  }

  async readField(step: OceanEngineFormPlan["steps"][number]) {
    return step.value;
  }

  async readMoneyConstraint() {
    return this.moneyConstraint;
  }

  async assertFinalReady() {}

  async clickFinal() {
    this.finalClicks += 1;
  }

  async observeSubmit() {
    return this.submitObservation;
  }

  async reconcileSubmit() {
    return this.reconciliation;
  }
}

function compilePromotionPlan() {
  const input = readJSON("docs/delivery/fixtures/oceanengine-promotion-create-plan-input-v1.json") as OceanEnginePlanCompilerInput;
  return compileOceanEngineFormPlan("promotion_create", manifest, input);
}

test("configuration plan set binds a Runner v3 form and displays its differences", () => {
  const plan = compilePromotionPlan();
  plan.account_reference = "1855554434276391";
  plan.parent_project_reference = "7677595885572784182";
  const value = {
    schema_version: "oceanengine-configuration-plan-set/v1",
    configuration_id: "configuration-1",
    configuration_hash: "a".repeat(64),
    account_reference: plan.account_reference,
    forms: [{
      internal_object_kind: "promotion",
      internal_object_id: "promotion-draft-1",
      platform_object_id: "7683558668450021382",
      plan,
      diff: [{ field_key: "promotion.call_to_action", operation: "configure_object", target: ["立即预订"] }],
    }],
  };
  assert.equal(validatePlanSet(value), true, JSON.stringify(validatePlanSet.errors));
});

test("runner v3 executes prepare fields and stops at the final-click boundary", async () => {
  const plan = compilePromotionPlan();
  const page = new FakePage();
  const result = await executePreparePlan(plan, page);
  assert.equal(validateResult(result), true, JSON.stringify(validateResult.errors));
  assert.equal(result.outcome, "success");
  assert.equal(result.final_click_performed, false);
  assert.equal(page.identified, 1);
  assert.ok(page.applied.includes("promotion.copy_materials"));
  assert.equal(result.steps.at(-1)?.status, "blocked_boundary");
});

test("a blocked compiled plan does not access the page", async () => {
  const plan = compilePromotionPlan();
  plan.status = "blocked";
  plan.blocked_reasons = ["missing_parent_reference:test"];
  const page = new FakePage();
  const result = await executePreparePlan(plan, page);
  assert.equal(result.outcome, "blocked");
  assert.equal(page.identified, 0);
  assert.deepEqual(page.applied, []);
});

test("runner v3 rejects any plan that enables a final click", async () => {
  const plan = compilePromotionPlan();
  const unsafe = { ...plan, allow_remote_write: true } as unknown as OceanEngineFormPlan;
  const result = await executePreparePlan(unsafe, new FakePage());
  assert.equal(result.outcome, "failed");
  assert.equal(result.error_code, "write_blocked");
  assert.equal(result.final_click_performed, false);
});

test("runner v3 rejects a missing blocked boundary", async () => {
  const plan = compilePromotionPlan();
  plan.steps = plan.steps.slice(0, -1);
  const result = await executePreparePlan(plan, new FakePage());
  assert.equal(result.outcome, "failed");
  assert.equal(result.error_code, "invalid_plan");
});

test("runner v3 keeps complex field values in readback", async () => {
  const plan = compilePromotionPlan();
  const page = new FakePage();
  const result = await executePreparePlan(plan, page);
  const copyStep = result.steps.find((step) => step.id.includes("promotion.copy_materials"));
  assert.deepEqual(copyStep?.readback, ["校准文案"]);
});

test("runner v3 stops when the visible bid constraint differs from the plan", async () => {
  const plan = compilePromotionPlan();
  const bid = plan.steps.find((step) => step.field_key === "promotion.bid");
  assert.ok(bid);
  bid.money_constraint = {
    schema_version: "oceanengine-bid-constraints/v1",
    charging_mode: "CPM",
    minimum_minor: 400,
    maximum_minor: 10000,
    maximum_source: "static",
  };
  const page = new FakePage();
  page.moneyConstraint = { minimum_minor: 1, maximum_minor: 1000000 };
  const result = await executePreparePlan(plan, page);
  assert.equal(result.outcome, "failed");
  assert.equal(result.error_code, "page_drift");
  assert.equal(result.final_click_performed, false);
});

test("parses visible OceanEngine money ranges", () => {
  assert.deepEqual(parseOceanEngineMoneyConstraint(["请输入", "出价范围 4～100 元"]), {
    minimum_minor: 400,
    maximum_minor: 10000,
  });
  assert.deepEqual(parseOceanEngineMoneyConstraint(["出价\n元\n请输入项目出价，不少于4元，不超过100元"]), {
    minimum_minor: 400,
    maximum_minor: 10000,
  });
  assert.deepEqual(parseOceanEngineMoneyConstraint(["请输入项目出价，不少于0.01元，不超过300元"]), {
    minimum_minor: 1,
    maximum_minor: 30000,
  });
  assert.equal(parseOceanEngineMoneyConstraint(["出价\n元"]), undefined);
  assert.equal(parseOceanEngineMoneyConstraint(["不少于100元，不超过4元"]), undefined);
  assert.equal(parseOceanEngineMoneyConstraint(["出价过低，同类单元出价在 11.57 ~ 15.35 元之间，建议提高出价"]), undefined);
  assert.deepEqual(parseOceanEngineMoneyConstraint(["建议出价 11.57 ~ 15.35 元\n请输入项目出价，不少于0.01元，不超过300元"]), { minimum_minor: 1, maximum_minor: 30000 });
});

test("runner v3 retains the live promotion adapters", () => {
	assert.match(runnerSource, /createad_nativetype_0/);
	assert.match(runnerSource, /label === "账户信息"/);
  assert.match(runnerSource, /promotion\.product_image_references/);
	assert.match(runnerSource, /promotion\.direct_link_reference/);
	assert.match(runnerSource, /createad_openUrl_input_input_component/);
	assert.match(runnerSource, /createad_productName/);
	assert.match(runnerSource, /waitForStableMaterialCard/);
	assert.match(runnerSource, /waitForStableMaterialInventory/);
	assert.match(runnerSource, /createad_videoLib_close/);
	assert.match(runnerSource, /createad_productImg__createProductImg__createMaterialLib/);
	assert.match(runnerSource, /可搜索视频名称或ID/);
	assert.match(runnerSource, /searched_material_card_and_object_id/);
  assert.match(runnerSource, /\.oc-create-product-img-add-button/);
  assert.match(runnerSource, /root\.locator\("img:visible"\)/);
  assert.match(runnerSource, /waitForStableProductImage/);
  assert.match(runnerSource, /stable_absolute_img_src_and_selected_count/);
  assert.match(runnerSource, /oc-create-material-card-selected:visible/);
  assert.doesNotMatch(runnerSource, /product image index is out of range/);
  assert.match(runnerSource, /createad_yuntuCategory_popover_content/);
  assert.match(runnerSource, /ovui-cascader-search-option/);
  assert.match(runnerSource, /自定义品牌名称/);
  assert.match(runnerSource, /await target\.press\("Enter"\)/);
  assert.match(runnerSource, /step\.operation === "toggle"/);
  assert.match(runnerSource, /checkboxState/);
  assert.match(runnerSource, /ovui-switch--checked/);
  assert.match(runnerSource, /createproject_aigcDynamic/);
});

test("project submission reconciles the selected external_action from the observed write request", () => {
  assert.match(runnerSource, /request\.postDataJSON\(\)/);
  assert.match(runnerSource, /payload\.external_action/);
  assert.match(runnerSource, /optimization_target_external_action/);
  assert.match(runnerSource, /reconcileProjectSubmission/);
});

test("delivery mode ignores hidden controls retained by a prior marketing branch", () => {
  assert.match(runnerSource, /step\.field_key === "project\.delivery_mode"/);
  assert.match(runnerSource, /createproject_deliverymode\$\{expectedSuffix\}/);
  assert.match(runnerSource, /Hidden controls from an old marketing branch must not match/);
});

test("product image matching ignores signed URL hosts, queries, and transform suffixes", () => {
  const webURI = "tos-cn-i-sd07hgqsbj/26d59f497e2540a3a377554cd94e4706";
  const signed = "https://p0-adplatform-private.oceanengine.com/tos-cn-i-sd07hgqsbj/26d59f497e2540a3a377554cd94e4706~tplv-iq460dd072-origin.image?sign_for=ad_platform&x-orig-sign=changed";
  assert.equal(canonicalImageSourceIdentity(webURI), webURI);
  assert.equal(canonicalImageSourceIdentity(signed), webURI);
  assert.equal(isStablePlatformImageSourceIdentity(webURI), true);
  assert.equal(isStablePlatformImageSourceIdentity("api/connector/v1/projects/project-1/platform-objects/image-1/preview"), false);
});

test("runner v3 enters the project form from the management page", () => {
	assert.match(runnerSource, /resolvePlanPage/);
	assert.match(runnerSource, /plan\.plan_kind === "promotion_create"/);
	assert.match(runnerSource, /searchParams\.set\("project_id", plan\.parent_project_reference/);
  assert.match(runnerSource, /getByText\("新建项目", \{ exact: true \}\)/);
  assert.match(runnerSource, /managementURL\.searchParams\.set\("aadvid", plan\.account_reference\)/);
  assert.match(runnerSource, /the project creation form did not load/);
  assert.match(runnerSource, /runner_step_start/);
});

test("runner v3 supports ecommerce and sales-lead product controls", () => {
  assert.match(runnerSource, /locator\("\.create-product-add-empty"\)/);
  assert.match(runnerSource, /\["点击选择商品", "添加商品"\]/);
  assert.match(runnerSource, /getByPlaceholder\("请输入商品名称或ID", \{ exact: true \}\)/);
  assert.match(runnerSource, /spec\.expected_total !== undefined && !projectProduct/);
  assert.match(runnerSource, /searched_product_card_and_unique_product_id/);
  assert.match(runnerSource, /waitForProductListRequest/);
  assert.match(runnerSource, /clue_product_list/);
  assert.match(runnerSource, /finishProductListRequest/);
  assert.match(runnerSource, /waitForStableProductCard/);
  assert.match(runnerSource, /createproject_productselectdrawer_close/);
  assert.match(runnerSource, /createproject_audienceextend_/);
  assert.doesNotMatch(runnerSource, /projectMoney\.nth\(step\.field_key === "project\.daily_budget" \? 0 : 1\)/);
  assert.match(runnerSource, /process\.stdout\.write\(JSON\.stringify\(result\)/);
  assert.match(runnerSource, /commandOption\("--result-file"\)/);
  assert.match(runnerSource, /writeFileSync\(resultFile, JSON\.stringify\(result\)\)/);
  assert.match(runnerSource, /writeResultAndExit\(result, 0\)/);
});

test("promotion reconciliation reads the full call-to-action multi-select", () => {
  assert.match(runnerSource, /promotion\.landing_page_reference/);
  assert.match(runnerSource, /promotion\.call_to_action/);
  assert.match(runnerSource, /selectedCallToActions/);
  assert.match(runnerSource, /call-to-action set does not match the plan/);
  assert.match(runnerSource, /\.ovui-tag__close/);
  assert.match(runnerSource, /locator\("tr\.ovui-tr"\)/);
  assert.doesNotMatch(runnerSource, /const previewTitle = editPage\.getByText\("单元素材预览"/);
});

test("native manual-title submit remains blocked before page access", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  preparePlan.steps.splice(1, 0, { id: "native-title-mode", kind: "field_action", field_key: "promotion.title_mode", operation: "choose_exact_visible_option", value: "手动添加", remote_write: false, blocked: false });
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, { account_reference: preparePlan.account_reference, maximum_money_cny: 300, schedule_date: "2026-08-26" }, now);
  const page = new FakePage();
  const result = await executePlan(bundle.plan, page, { confirmToken: bundle.confirm_token, now });
  assert.equal(result.outcome, "blocked");
  assert.equal(result.error_code, "native_promotion_submit_not_calibrated");
  assert.equal(result.final_click_performed, false);
  assert.equal(page.finalClicks, 0);
  assert.deepEqual(result.steps, []);
});

function nativePromotionPlan() {
  const plan = compilePromotionPlan();
  plan.account_reference = "9000000000000000001";
  plan.parent_project_reference = "9000000000000000100";
  plan.steps = [plan.steps[0],
    { id: "video", kind: "field_action", field_key: "promotion.base_materials", value: { selection_kind: "async_row", material_type: "douyin_video", object_id: "9000000000000000201" }, remote_write: false, blocked: false },
    { id: "mode", kind: "field_action", field_key: "promotion.title_mode", value: "投放原视频标题", remote_write: false, blocked: false },
    { id: "name", kind: "field_action", field_key: "promotion.promotion_name", value: "Native promotion fixture", remote_write: false, blocked: false },
    plan.steps.at(-1)!,
  ];
  return plan;
}

const nativePromotionID = "9000000000000000101";
const nativeDetailFixture = () => readJSON("docs/delivery/fixtures/oceanengine-native-promotion-detail-v1.json");

test("native reconciliation matches saved aweme item IDs and original-title mode", () => {
  const result = reconcileNativePromotionDetail(nativePromotionPlan(), nativePromotionID, nativeDetailFixture());
  assert.equal(result.status, "matched");
  assert.deepEqual(result.fields[0].observed, ["9000000000000000201"]);
  assert.equal(result.fields[1].observed, "投放原视频标题");
});

test("native reconciliation fails closed for unavailable or unrelated details", () => {
  const plan = nativePromotionPlan();
  for (const payload of [undefined, null, [], {}, { code: 40102 }, { code: 0, data: {} }]) {
    assert.equal(reconcileNativePromotionDetail(plan, nativePromotionID, payload).status, "not_checked");
  }
  for (const field of ["id", "advertiser_id", "project_id", "name", "is_del", "project_is_del"]) {
    const payload = nativeDetailFixture();
    payload.data[nativePromotionID][field] = field.endsWith("is_del") ? 1 : "different";
    assert.equal(reconcileNativePromotionDetail(plan, nativePromotionID, payload).status, "not_checked", field);
  }
  for (const videos of [undefined, {}, [{ video_id: "9000000000000000201" }], [{ aweme_item_id: 9000000000000000201 }]]) {
    const payload = nativeDetailFixture();
    payload.data[nativePromotionID].material_group.video_material_info = videos;
    assert.equal(reconcileNativePromotionDetail(plan, nativePromotionID, payload).status, "not_checked");
  }
});

test("native reconciliation detects missing, extra, wrong videos and changed title mode", () => {
  for (const ids of [[], ["999"], ["9000000000000000201", "999"], ["9000000000000000201", "9000000000000000201"]]) {
    const payload = nativeDetailFixture();
    payload.data[nativePromotionID].material_group.video_material_info = ids.map(aweme_item_id => ({ aweme_item_id }));
    assert.equal(reconcileNativePromotionDetail(nativePromotionPlan(), nativePromotionID, payload).status, "drifted");
  }
  const payload = nativeDetailFixture();
  payload.data[nativePromotionID].data.native_info.origin_item_title_switch = 0;
  assert.equal(reconcileNativePromotionDetail(nativePromotionPlan(), nativePromotionID, payload).status, "drifted");
  delete payload.data[nativePromotionID].data.native_info.origin_item_title_switch;
  assert.equal(reconcileNativePromotionDetail(nativePromotionPlan(), nativePromotionID, payload).status, "not_checked");
});

test("native reconciliation checks persisted fields for response IDs, URL IDs and edits", async () => {
  for (const route of ["response", "url", "edit"] as const) {
    const plan = nativePromotionPlan();
    if (route === "edit") { plan.plan_kind = "promotion_edit"; plan.object_reference = nativePromotionID; }
    const requests: string[] = [];
    const page = {
      url: () => `https://ad.oceanengine.com/superior/ads?aadvid=${plan.account_reference}&promotion_id=${nativePromotionID}`,
      request: { get: async (url: string) => { requests.push(url); return { ok: () => true, json: async () => nativeDetailFixture() }; } },
    } as unknown as Page;
    const result = await new PlaywrightPageOperations(page).reconcileSubmit(plan, { outcome: "success", ...(route === "response" ? { created_object_id: nativePromotionID } : {}) });
    assert.equal(result.field_reconciliation?.status, "matched", route);
    assert.equal(requests.length, 1);
    const requestURL = new URL(requests[0]);
    assert.equal(requestURL.pathname, "/superior/api/ad/promotion/detail");
    assert.equal(requestURL.searchParams.get("promotion_ids"), nativePromotionID);
    assert.equal(requestURL.searchParams.get("aadvid"), plan.account_reference);
  }
});

test("native detail outages preserve the discovered ID without claiming success", async () => {
  let attempts = 0;
  const page = {
    url: () => "https://ad.oceanengine.com/promotion/promote-manage/ad",
    request: { get: async () => { attempts += 1; throw new Error("detail unavailable"); } },
    waitForTimeout: async () => {},
  } as unknown as Page;
  const result = await new PlaywrightPageOperations(page).reconcileSubmit(nativePromotionPlan(), { outcome: "success", created_object_id: nativePromotionID });
  assert.equal(result.created_object_id, nativePromotionID);
  assert.equal(result.field_reconciliation?.status, "not_checked");
  assert.equal(attempts, 3);
});

test("native submit uses one authority and requires persisted field evidence", async () => {
  for (const checked of [true, false]) {
    const plan = nativePromotionPlan();
    const now = new Date("2026-09-10T01:00:00Z");
    const bundle = authorizeSubmitPlan(plan, { account_reference: plan.account_reference, maximum_money_cny: 300, schedule_date: "2026-09-11" }, now);
    const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-native-authority-"));
    try {
      const page = new FakePage();
      page.reconciliation = { status: "matched", created_object_id: nativePromotionID,
        ...(checked ? { field_reconciliation: reconcileNativePromotionDetail(plan, nativePromotionID, nativeDetailFixture()) } : {}) };
      const result = await executePlan(bundle.plan, page, { confirmToken: bundle.confirm_token, authorityStateDirectory: stateDirectory, now });
      assert.equal(result.outcome, checked ? "success" : "result_unknown");
      assert.equal(result.created_object_id, nativePromotionID);
      assert.equal(page.finalClicks, 1);
      assert.equal(validateResult(result), true, JSON.stringify(validateResult.errors));
      await executePlan(bundle.plan, page, { confirmToken: bundle.confirm_token, authorityStateDirectory: stateDirectory, now });
      assert.equal(page.finalClicks, 1);
    } finally { await rm(stateDirectory, { recursive: true, force: true }); }
  }
});

test("runner v3 consumes one authority and performs one final click", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-runner-authority-"));
  try {
    const page = new FakePage();
    const result = await executePlan(bundle.plan, page, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(validateResult(result), true, JSON.stringify(validateResult.errors));
    assert.equal(result.outcome, "success");
    assert.equal(result.final_click_performed, true);
    assert.equal(result.created_object_id, "7677604041052405801");
    assert.equal(result.reconciliation, "matched");
    assert.equal(page.finalClicks, 1);

    const retryPage = new FakePage();
    const retry = await executePlan(bundle.plan, retryPage, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(retry.outcome, "failed");
    assert.equal(retry.error_code, "authority_already_used");
    assert.equal(retryPage.finalClicks, 0);
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});

test("runner v3 reports a successful create with persisted field drift", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-runner-drift-"));
  try {
    const page = new FakePage();
    page.reconciliation = {
      status: "matched",
      created_object_id: "7677604041052405801",
      field_reconciliation: {
        status: "drifted",
        fields: [{
          field_key: "promotion.landing_page_reference",
          expected: "7545332540006350875",
          observed: "7667531743012438066",
          status: "drifted",
        }],
      },
    };
    const result = await executePlan(bundle.plan, page, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(validateResult(result), true, JSON.stringify(validateResult.errors));
    assert.equal(result.outcome, "success_with_drift");
    assert.equal(result.error_code, "field_reconciliation_drift");
    assert.equal(result.field_reconciliation?.status, "drifted");
    assert.equal(page.finalClicks, 1);
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});

test("runner v3 stops when required persisted fields cannot be checked", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-runner-not-checked-"));
  try {
    const page = new FakePage();
    page.reconciliation = {
      status: "matched",
      created_object_id: "7677604041052405801",
      field_reconciliation: {
        status: "not_checked",
        fields: [
          { field_key: "promotion.landing_page_reference", expected: "7545332540006350875", status: "not_checked" },
          { field_key: "promotion.call_to_action", expected: ["立即预订"], status: "not_checked" },
        ],
      },
    };
    const result = await executePlan(bundle.plan, page, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(validateResult(result), true, JSON.stringify(validateResult.errors));
    assert.equal(result.outcome, "result_unknown");
    assert.equal(result.error_code, "field_reconciliation_not_checked");
    assert.equal(result.created_object_id, "7677604041052405801");
    assert.equal(result.field_reconciliation?.status, "not_checked");
    assert.equal(page.finalClicks, 1);
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});

test("runner v3 does not click when the confirmation token is wrong", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const page = new FakePage();
  const result = await executePlan(bundle.plan, page, {
    confirmToken: "incorrect-token",
    authorityStateDirectory: join(tmpdir(), "cookies-authority-unused"),
    now,
  });
  assert.equal(result.outcome, "failed");
  assert.equal(result.error_code, "authority_token_mismatch");
  assert.equal(page.finalClicks, 0);
});

test("runner v3 reconciles an unclear submit without a second click", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-runner-unknown-"));
  try {
    const page = new FakePage();
    page.submitObservation = { outcome: "result_unknown", error_message: "no stable result" };
    const result = await executePlan(bundle.plan, page, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(result.outcome, "success");
    assert.equal(result.final_click_performed, true);
    assert.equal(page.finalClicks, 1);
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});

test("runner v3 reports a confirmed no-effect failure when no write request and no target exist", async () => {
  const preparePlan = compilePromotionPlan();
  preparePlan.account_reference = "1855554434276391";
  preparePlan.parent_project_reference = "7677595885572784182";
  const now = new Date("2026-08-25T01:00:00.000Z");
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-runner-no-effect-"));
  try {
    const page = new FakePage();
    page.submitObservation = {
      outcome: "result_unknown",
      error_message: "no stable result",
      platform_write_request_observed: false,
    };
    page.reconciliation = { status: "not_found" };
    const result = await executePlan(bundle.plan, page, {
      confirmToken: bundle.confirm_token,
      authorityStateDirectory: stateDirectory,
      now,
    });
    assert.equal(result.outcome, "failed");
    assert.equal(result.error_code, "submit_no_effect_confirmed");
    assert.equal(result.reconciliation, "not_found");
    assert.equal(result.final_click_performed, true);
    assert.equal(page.finalClicks, 1);
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});
