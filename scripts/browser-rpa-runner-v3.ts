import { chromium, type BrowserContext, type Locator, type Page, type Request } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

import type { OceanEngineFormPlan } from "./oceanengine-form-plan-compiler.ts";
import { resolveSessionPlaywrightEndpoint } from "./browser-rpa-edge-session.ts";
import {
  AuthorityError,
  consumeSubmitAuthority,
  validateSubmitAuthority,
} from "./rpa-authority.ts";

type PlanStep = OceanEngineFormPlan["steps"][number];

export type ReferenceSelectionSpec = {
  selection_kind: "text_option" | "async_row" | "media_card" | "image_card";
  label?: string;
  object_id?: string;
  index?: number;
  minimum_visible?: number;
  expected_total?: number;
  confirm_button?: string;
  image_src_identity?: string;
};

export function canonicalImageSourceIdentity(value: string) {
  if (!value.trim()) return "";
  try {
    const parsed = new URL(value.trim(), "https://cookies.invalid");
    const path = decodeURIComponent(parsed.pathname).replace(/^\/+/, "");
    return path.split("~", 1)[0];
  } catch {
    return "";
  }
}

export type RunnerStepResult = {
  id: string;
  status: "succeeded" | "blocked_boundary" | "submitted" | "result_unknown" | "failed";
  readback?: unknown;
  error_code?: string;
  error_message?: string;
  page_reference?: string;
};

export type RunnerV3Result = {
  schema_version: "oceanengine-playwright-rpa-result/v2";
  outcome: "success" | "success_with_drift" | "blocked" | "failed" | "result_unknown";
  error_code: string;
  error_message?: string;
  final_click_performed: boolean;
  created_object_id?: string;
  reconciliation?: "not_started" | "matched" | "not_found" | "not_applicable";
  field_reconciliation?: FieldReconciliation;
  steps: RunnerStepResult[];
};

export type FieldReconciliation = {
  status: "matched" | "drifted" | "not_checked";
  fields: Array<{
    field_key: string;
    expected?: string | string[];
    observed?: string | string[];
    status: "matched" | "drifted" | "not_checked";
  }>;
};

export type SubmitObservation = {
  outcome: "success" | "validation_error" | "result_unknown";
  error_message?: string;
  created_object_id?: string;
  platform_write_request_observed?: boolean;
  platform_write_response_status?: number;
};

export type ReconciliationResult = {
  status: "matched" | "not_found" | "not_applicable";
  created_object_id?: string;
  field_reconciliation?: FieldReconciliation;
  query_attempts?: number;
  exact_name_matches?: number;
};

export interface PageOperations {
  identifyPage(plan: OceanEngineFormPlan): Promise<void>;
  applyField(step: PlanStep): Promise<void>;
  readField(step: PlanStep): Promise<unknown>;
  readMoneyConstraint?(step: PlanStep): Promise<{ minimum_minor: number; maximum_minor: number } | undefined>;
  assertFinalReady(step: PlanStep): Promise<void>;
  clickFinal(step: PlanStep): Promise<void>;
  observeSubmit(plan: OceanEngineFormPlan): Promise<SubmitObservation>;
  reconcileSubmit(plan: OceanEngineFormPlan, observation: SubmitObservation): Promise<ReconciliationResult>;
}

export function isStablePlatformImageSourceIdentity(value: string) {
  const identity = canonicalImageSourceIdentity(value);
  return Boolean(identity) && !identity.startsWith("api/");
}

export function parseOceanEngineMoneyConstraint(values: readonly string[]) {
  let minimum: number | undefined;
  let maximum: number | undefined;
  for (const value of values) {
    const match = value.match(/(\d+(?:\.\d+)?)\s*(?:-|~|～|至|到)\s*(\d+(?:\.\d+)?)/);
    if (!match) continue;
    minimum = Math.round(Number(match[1]) * 100);
    maximum = Math.round(Number(match[2]) * 100);
    if (Number.isFinite(minimum) && Number.isFinite(maximum) && maximum >= minimum) break;
    minimum = undefined;
    maximum = undefined;
  }
  return minimum !== undefined && maximum !== undefined
    ? { minimum_minor: minimum, maximum_minor: maximum }
    : undefined;
}

class RunnerV3Error extends Error {
  constructor(readonly code: string, message: string) {
    super(message);
  }
}

function validatePreparePlan(plan: OceanEngineFormPlan) {
  if (plan.schema_version !== "oceanengine-playwright-rpa-plan/v3") {
    throw new RunnerV3Error("invalid_plan", "unsupported plan schema");
  }
  if (plan.browser !== "msedge" || plan.mode !== "prepare") {
    throw new RunnerV3Error("invalid_plan", "runner v3 accepts Edge prepare plans only");
  }
  if (plan.allow_remote_write || plan.maximum_final_clicks !== 0) {
    throw new RunnerV3Error("write_blocked", "prepare plan cannot authorize a final click");
  }
  const boundary = plan.steps.at(-1);
  if (
    boundary?.kind !== "final_click_boundary" ||
    !boundary.remote_write ||
    !boundary.blocked ||
    boundary.block_reason !== "PREPARE_PLAN_REMOTE_WRITE_PROHIBITED"
  ) {
    throw new RunnerV3Error("invalid_plan", "prepare plan has no blocked final-click boundary");
  }
}

function validateSubmitPlan(plan: OceanEngineFormPlan, confirmToken: string | undefined, now: Date) {
  if (plan.schema_version !== "oceanengine-playwright-rpa-plan/v3" || plan.browser !== "msedge") {
    throw new RunnerV3Error("invalid_plan", "runner v3 accepts Edge v3 plans only");
  }
  if (!confirmToken) throw new RunnerV3Error("authority_missing", "submit mode needs --confirm-token");
  try {
    return validateSubmitAuthority(plan, confirmToken, now);
  } catch (error) {
    if (error instanceof AuthorityError) throw new RunnerV3Error(error.code, error.message);
    throw error;
  }
}

function stableReferenceID(value: unknown) {
  if (typeof value === "string" && value.trim()) return value.trim();
  if (!value || typeof value !== "object") return undefined;
  const record = value as Record<string, unknown>;
  for (const key of ["object_id", "objectId", "id"]) {
    if (typeof record[key] === "string" && /^\d+$/.test(record[key])) return record[key] as string;
  }
  return undefined;
}

export type ExecutePlanOptions = {
  confirmToken?: string;
  authorityStateDirectory?: string;
  now?: Date;
  onStepStart?: (step: PlanStep) => void;
};

export async function executePlan(
  plan: OceanEngineFormPlan,
  page: PageOperations,
  options: ExecutePlanOptions = {},
): Promise<RunnerV3Result> {
  const results: RunnerStepResult[] = [];
  let finalClickPerformed = false;
  try {
    const now = options.now ?? new Date();
    const authority = plan.mode === "prepare"
      ? (validatePreparePlan(plan), undefined)
      : validateSubmitPlan(plan, options.confirmToken, now);
    if (plan.status === "blocked") {
      return {
        schema_version: "oceanengine-playwright-rpa-result/v2",
        outcome: "blocked",
        error_code: "plan_blocked",
        error_message: plan.blocked_reasons.join(","),
        final_click_performed: false,
        reconciliation: "not_started",
        steps: results,
      };
    }

    const fieldSteps: PlanStep[] = [];
    for (const step of plan.steps) {
      options.onStepStart?.(step);
      if (step.kind === "identify_page") {
        await page.identifyPage(plan);
        results.push({ id: step.id, status: "succeeded" });
      } else if (step.kind === "field_action") {
        if (step.value_state === "missing") {
          results.push({ id: step.id, status: "succeeded", readback: null });
          continue;
        }
        await page.applyField(step);
        if (step.money_constraint) {
          const observed = await page.readMoneyConstraint?.(step);
          if (!observed) {
            throw new RunnerV3Error("page_drift", `${step.id}: the visible bid constraint is unavailable`);
          }
          if (
            observed.minimum_minor !== step.money_constraint.minimum_minor
            || observed.maximum_minor !== step.money_constraint.maximum_minor
          ) {
            throw new RunnerV3Error(
              "page_drift",
              `${step.id}: bid constraint drift; expected ${step.money_constraint.minimum_minor}-${step.money_constraint.maximum_minor} minor, observed ${observed.minimum_minor}-${observed.maximum_minor} minor`,
            );
          }
        }
        fieldSteps.push(step);
        results.push({ id: step.id, status: "succeeded", readback: await page.readField(step) });
      } else if (step.kind === "readback") {
        const readback: Record<string, unknown> = {};
        for (const field of fieldSteps) {
          if (field.field_key) readback[field.field_key] = await page.readField(field);
        }
        results.push({ id: step.id, status: "succeeded", readback });
      } else if (step.kind === "final_click_boundary") {
        if (plan.mode === "prepare") {
          results.push({ id: step.id, status: "blocked_boundary", error_code: step.block_reason });
          continue;
        }
        if (!authority) throw new RunnerV3Error("authority_missing", "submit authority is unavailable");
        const stateDirectory = options.authorityStateDirectory;
        if (!stateDirectory) throw new RunnerV3Error("authority_state_missing", "submit mode needs an authority state directory");
        await page.assertFinalReady(step);
        try {
          await consumeSubmitAuthority(authority, stateDirectory);
        } catch (error) {
          if (error instanceof AuthorityError) throw new RunnerV3Error(error.code, error.message);
          throw error;
        }
        await page.clickFinal(step);
        finalClickPerformed = true;
        const observation = await page.observeSubmit(plan);
        if (observation.outcome === "validation_error") {
          results.push({ id: step.id, status: "failed", error_code: "submit_validation_error", error_message: observation.error_message });
          return {
            schema_version: "oceanengine-playwright-rpa-result/v2",
            outcome: "failed",
            error_code: "submit_validation_error",
            error_message: observation.error_message,
            final_click_performed: true,
            reconciliation: "not_started",
            steps: results,
          };
        }
        const reconciliation = await page.reconcileSubmit(plan, observation);
        if (reconciliation.status === "not_found") {
          const noWriteRequest = observation.platform_write_request_observed === false;
          const readback = {
            ...reconciliation,
            platform_write_request_observed: observation.platform_write_request_observed,
            ...(observation.platform_write_response_status !== undefined
              ? { platform_write_response_status: observation.platform_write_response_status }
              : {}),
          };
          if (observation.outcome === "result_unknown" && noWriteRequest) {
            results.push({ id: step.id, status: "failed", error_code: "submit_no_effect_confirmed", readback });
            return {
              schema_version: "oceanengine-playwright-rpa-result/v2",
              outcome: "failed",
              error_code: "submit_no_effect_confirmed",
              error_message: "the final click sent no platform write request and exact-name reconciliation found no target object",
              final_click_performed: true,
              reconciliation: "not_found",
              steps: results,
            };
          }
          results.push({ id: step.id, status: "result_unknown", error_code: "reconciliation_not_found", readback: reconciliation });
          return {
            schema_version: "oceanengine-playwright-rpa-result/v2",
            outcome: "result_unknown",
            error_code: "reconciliation_not_found",
            error_message: "the submit succeeded but exact-name ID reconciliation did not find one row",
            final_click_performed: true,
            reconciliation: "not_found",
            steps: results,
          };
        }
        results.push({ id: step.id, status: "submitted", readback: reconciliation });
        const fieldStatus = reconciliation.field_reconciliation?.status;
        if (fieldStatus === "not_checked") {
          return {
            schema_version: "oceanengine-playwright-rpa-result/v2",
            outcome: "result_unknown",
            error_code: "field_reconciliation_not_checked",
            error_message: "the object ID was found but required persisted fields could not be checked",
            final_click_performed: true,
            ...(reconciliation.created_object_id ? { created_object_id: reconciliation.created_object_id } : {}),
            reconciliation: reconciliation.status,
            field_reconciliation: reconciliation.field_reconciliation,
            steps: results,
          };
        }
        const fieldDrifted = fieldStatus === "drifted";
        return {
          schema_version: "oceanengine-playwright-rpa-result/v2",
          outcome: fieldDrifted ? "success_with_drift" : "success",
          error_code: fieldDrifted ? "field_reconciliation_drift" : "ok",
          ...(fieldDrifted ? { error_message: "the object was created but one or more persisted fields differ from the submitted values" } : {}),
          final_click_performed: true,
          ...(reconciliation.created_object_id ? { created_object_id: reconciliation.created_object_id } : {}),
          reconciliation: reconciliation.status,
          ...(reconciliation.field_reconciliation ? { field_reconciliation: reconciliation.field_reconciliation } : {}),
          steps: results,
        };
      } else {
        throw new RunnerV3Error("invalid_plan", `unsupported step kind: ${step.kind}`);
      }
    }

    return {
      schema_version: "oceanengine-playwright-rpa-result/v2",
      outcome: "success",
      error_code: "ok",
      final_click_performed: false,
      reconciliation: "not_started",
      steps: results,
    };
  } catch (error) {
    const code = error instanceof RunnerV3Error ? error.code : finalClickPerformed ? "result_unknown" : "execution_failed";
    const message = error instanceof Error ? error.message : String(error);
    return {
      schema_version: "oceanengine-playwright-rpa-result/v2",
      outcome: finalClickPerformed ? "result_unknown" : "failed",
      error_code: code,
      error_message: message,
      final_click_performed: finalClickPerformed,
      reconciliation: "not_started",
      steps: results,
    };
  }
}

async function waitForPlanPage(
  context: BrowserContext,
  plan: OceanEngineFormPlan,
  timeoutMs: number,
) {
  const expectedPath = plan.plan_kind.startsWith("project_") ? "/superior/create-project" : "/superior/ads";
  const objectParameter = plan.plan_kind.startsWith("project_") ? "project_id" : "promotion_id";
  const deadline = Date.now() + timeoutMs;
  do {
    const page = context.pages().find((candidate) => {
      try {
        const url = new URL(candidate.url());
        if (url.hostname !== "ad.oceanengine.com" || !url.pathname.startsWith(expectedPath)) return false;
        if (!plan.object_reference || plan.object_reference.startsWith("redacted:")) return true;
        return url.searchParams.get(objectParameter) === plan.object_reference;
      } catch {
        return false;
      }
    });
    if (page) return page;
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 250));
  } while (Date.now() < deadline);
  return undefined;
}

async function resolvePlanPage(context: BrowserContext, plan: OceanEngineFormPlan) {
  if (
    plan.plan_kind === "promotion_create" &&
    /^\d+$/.test(plan.account_reference) &&
    /^\d+$/.test(plan.parent_project_reference ?? "")
  ) {
    const createPage = await waitForPlanPage(context, plan, 3_000) ?? await context.newPage();
    const createURL = new URL("https://ad.oceanengine.com/superior/ads");
    createURL.searchParams.set("aadvid", plan.account_reference);
    createURL.searchParams.set("project_id", plan.parent_project_reference!);
    await createPage.goto(createURL.toString(), { waitUntil: "domcontentloaded" });
    const opened = await waitForPlanPage(context, plan, 15_000);
    if (opened) return opened;
    await createPage.close().catch(() => undefined);
    throw new RunnerV3Error("async_load_timeout", "the promotion creation form did not load");
  }

  const existing = await waitForPlanPage(context, plan, 3_000);
  if (existing) return existing;

  const manageDeadline = Date.now() + 10_000;
  let managePage: Page | undefined;
  do {
    managePage = context.pages().find((candidate) => {
      try {
        const url = new URL(candidate.url());
        return url.hostname === "ad.oceanengine.com"
          && url.pathname.startsWith("/promotion/promote-manage/project")
          && (plan.account_reference.startsWith("redacted:") || url.searchParams.get("aadvid") === plan.account_reference);
      } catch {
        return false;
      }
    });
    if (!managePage) await new Promise((resolvePromise) => setTimeout(resolvePromise, 250));
  } while (!managePage && Date.now() < manageDeadline);

  if (!managePage && plan.plan_kind === "project_create" && /^\d+$/.test(plan.account_reference)) {
    managePage = await context.newPage();
    const managementURL = new URL("https://ad.oceanengine.com/promotion/promote-manage/project");
    managementURL.searchParams.set("aadvid", plan.account_reference);
    await managePage.goto(managementURL.toString(), { waitUntil: "domcontentloaded" });
  }

  if (!managePage) throw new RunnerV3Error("page_drift", "the OceanEngine management page did not load");
  if (plan.plan_kind !== "project_create") {
    throw new RunnerV3Error("operator_required", `open the ${plan.plan_kind} form before Prepare`);
  }

  const createProjectCandidates = managePage.getByText("新建项目", { exact: true });
  let createProject: Locator | undefined;
  for (let attempt = 0; attempt < 120 && !createProject; attempt += 1) {
    const visible: Locator[] = [];
    for (let index = 0; index < await createProjectCandidates.count(); index += 1) {
      const candidate = createProjectCandidates.nth(index);
      if (await candidate.isVisible()) visible.push(candidate);
    }
    if (visible.length > 1) {
      throw new RunnerV3Error("locator_not_unique", "the New Project action is not unique");
    }
    createProject = visible[0];
    if (!createProject) await managePage.waitForTimeout(250);
  }
  if (!createProject) throw new RunnerV3Error("async_load_timeout", "the New Project action did not load");
  await createProject.click({ noWaitAfter: true });
  const opened = await waitForPlanPage(context, plan, 15_000);
  if (!opened) throw new RunnerV3Error("async_load_timeout", "the project creation form did not load");
  return opened;
}

export async function executePreparePlan(
  plan: OceanEngineFormPlan,
  page: PageOperations,
) {
  return executePlan(plan, page);
}

export class PlaywrightPageOperations implements PageOperations {
  private preSubmitUrl = "";
  private platformWriteRequestObserved = false;
  private platformWriteResponseStatus: number | undefined;
  private submittedExternalAction: string | undefined;
  private readonly referenceReadbacks = new Map<string, unknown>();

  constructor(private readonly page: Page) {}

  async identifyPage(plan: OceanEngineFormPlan) {
    const url = new URL(this.page.url());
    if (url.protocol !== "https:" || url.hostname !== "ad.oceanengine.com") {
      throw new RunnerV3Error("page_drift", "the active page is not an OceanEngine HTTPS page");
    }
    const expectedPath = plan.plan_kind.startsWith("project_") ? "/superior/create-project" : "/superior/ads";
    if (!url.pathname.startsWith(expectedPath)) {
      throw new RunnerV3Error("page_drift", `expected ${expectedPath}, got ${url.pathname}`);
    }
    const observedAccount = url.searchParams.get("aadvid");
    if (
      observedAccount &&
      !plan.account_reference.startsWith("redacted:") &&
      observedAccount !== plan.account_reference
    ) {
      throw new RunnerV3Error("account_mismatch", "the active account does not match the plan");
    }
    const observedObject = url.searchParams.get(plan.plan_kind.startsWith("project_") ? "project_id" : "promotion_id");
    if (plan.object_reference && !plan.object_reference.startsWith("redacted:") && observedObject && observedObject !== plan.object_reference) {
      throw new RunnerV3Error("object_mismatch", "the active object does not match the plan");
    }
    const staleProductDrawer = this.page.locator("[data-e2e='createproject_productselectdrawer']:visible");
    for (let attempt = 0; attempt < 3 && (await staleProductDrawer.count()) > 0; attempt += 1) {
      const close = staleProductDrawer.last().locator("[data-e2e='createproject_productselectdrawer_close']");
      if ((await close.count()) !== 1 || !(await close.isVisible())) break;
      await close.click();
      await this.page.waitForTimeout(300);
    }
    if ((await staleProductDrawer.count()) > 0) {
      throw new RunnerV3Error("page_drift", "the stale product picker could not be closed");
    }
    const staleVideoClose = this.page.locator("[data-e2e='createad_videoLib_close']:visible");
    for (let attempt = 0; attempt < 3 && (await staleVideoClose.count()) > 0; attempt += 1) {
      if ((await staleVideoClose.count()) !== 1) break;
      await staleVideoClose.click();
      await this.page.waitForTimeout(300);
    }
    if ((await staleVideoClose.count()) > 0) {
      throw new RunnerV3Error("page_drift", "the stale video picker could not be closed");
    }
    const staleProductImageDrawer = this.page.locator("[data-e2e='createad_productImg__createProductImg__createMaterialLib']:visible");
    for (let attempt = 0; attempt < 3 && (await staleProductImageDrawer.count()) > 0; attempt += 1) {
      const close = staleProductImageDrawer.locator("[data-auto-id='drawer-close-btn'],.oc-drawer-close").first();
      if ((await close.count()) !== 1 || !(await close.isVisible())) break;
      await close.click();
      await this.page.waitForTimeout(300);
    }
    if ((await staleProductImageDrawer.count()) > 0) {
      throw new RunnerV3Error("page_drift", "the stale product image picker could not be closed");
    }
  }

  private scopeLocator(step: PlanStep) {
    if (!step.scope) throw new RunnerV3Error("invalid_plan", `${step.id}: scope is required`);
    return this.page.getByText(step.scope, { exact: true });
  }

  private async selectedCallToActions(page: Page = this.page) {
    let scope = page.locator("[data-e2e='createad_actionText__createRecommendTag']:visible");
    for (let attempt = 0; attempt < 40 && (await scope.count()) === 0; attempt += 1) {
      await page.waitForTimeout(250);
      scope = page.locator("[data-e2e='createad_actionText__createRecommendTag']:visible");
    }
    if ((await scope.count()) !== 1) return [];
    const tags = scope.locator(".oc-tag-text");
    const values: string[] = [];
    for (let index = 0; index < await tags.count(); index += 1) {
      const tag = tags.nth(index);
      const value = (await tag.innerText()).trim();
      if (value && !values.includes(value)) values.push(value);
    }
    return values;
  }

  private xpathLiteral(value: string) {
    if (!value.includes("'")) return `'${value}'`;
    if (!value.includes('"')) return `"${value}"`;
    return `concat(${value.split("'").map((part) => `'${part}'`).join(", \"'\", ")})`;
  }

  private async targetLocator(step: PlanStep): Promise<Locator> {
    if (!step.target) throw new RunnerV3Error("invalid_plan", `${step.id}: target is required`);
    if (step.field_key === "promotion.direct_link_reference" && step.operation === "fill_text") {
      let directLinkInput = this.page.locator("[data-e2e='createad_openUrl_input_input_component'] input:visible");
      for (let attempt = 0; attempt < 40 && (await directLinkInput.count()) === 0; attempt += 1) {
        await this.page.waitForTimeout(250);
        directLinkInput = this.page.locator("[data-e2e='createad_openUrl_input_input_component'] input:visible");
      }
      if ((await directLinkInput.count()) === 1) return directLinkInput;
      throw new RunnerV3Error("locator_not_unique", `${step.id}: direct-link input is not unique`);
    }
    if (step.field_key === "project.budget_mode") {
      // Sales-lead pages render the budget options without the ecommerce
      // "项目日预算" label. The exact option is stable across both branches.
      return this.uniqueVisibleText(step.target, step.id);
    }
    const scope = this.scopeLocator(step);
    for (let attempt = 0; attempt < 40 && (await scope.count()) < 1; attempt += 1) {
      await this.page.waitForTimeout(250);
    }
    if ((await scope.count()) < 1) throw new RunnerV3Error("page_drift", `${step.id}: scope did not load`);
    if (step.field_key === "project.marketing_product_reference") {
      const emptyProduct = this.page.locator(".create-product-add-empty");
      const visibleEmptyProducts: Locator[] = [];
      for (let index = 0; index < await emptyProduct.count(); index += 1) {
        if (await emptyProduct.nth(index).isVisible()) visibleEmptyProducts.push(emptyProduct.nth(index));
      }
      if (visibleEmptyProducts.length === 1) return visibleEmptyProducts[0];
      // The sales-lead branch does not use the ecommerce empty-product card.
      // It renders a text action instead. Use the branch-specific action before
      // falling back to the target from an older generated plan (usually 更换).
      for (const label of ["点击选择商品", "添加商品"]) {
        const candidates = this.page.getByText(label, { exact: true });
        const visible: Locator[] = [];
        for (let index = 0; index < await candidates.count(); index += 1) {
          if (await candidates.nth(index).isVisible()) visible.push(candidates.nth(index));
        }
        if (visible.length === 1) return visible[0];
        if (visible.length > 1) {
          throw new RunnerV3Error("locator_not_unique", `${step.id}: sales-lead product action is not unique`);
        }
      }
      return this.uniqueVisibleText(step.target, step.id);
    }
    if (step.field_key === "promotion.base_materials") {
      const addVideo = this.page.getByRole("button", { name: "添加视频", exact: true });
      if ((await addVideo.count()) === 1 && await addVideo.isVisible()) return addVideo;
    }
    if (step.field_key === "promotion.product_image_references") {
      const productImageScope = this.page.getByText("产品主图", { exact: true });
      const productImageRow = productImageScope.first().locator(
        "xpath=ancestor::div[contains(concat(' ',normalize-space(@class),' '),' oc-row ')][1]",
      );
      const addProductImage = productImageRow.locator(".oc-create-product-img-add-button");
      if ((await addProductImage.count()) === 1 && await addProductImage.isVisible()) return addProductImage;
    }
    if (step.field_key === "promotion.product_name") {
      const candidates = this.page.locator("[data-e2e='createad_productName'] input[placeholder='请输入']");
      const editable: Locator[] = [];
      for (let index = 0; index < await candidates.count(); index += 1) {
        const candidate = candidates.nth(index);
        if (await candidate.isVisible() && await candidate.isEnabled()) editable.push(candidate);
      }
      if (editable.length === 1) return editable[0];
      throw new RunnerV3Error("locator_not_unique", `${step.id}: editable product name is not unique`);
    }
    if (step.field_key === "promotion.promotion_name") {
      const candidates = this.page.getByPlaceholder("请输入", { exact: true });
      const editable: Locator[] = [];
      for (let index = 0; index < await candidates.count(); index += 1) {
        const candidate = candidates.nth(index);
        if (await candidate.isVisible() && await candidate.isEnabled()) editable.push(candidate);
      }
      if (editable.length > 0) return editable.at(-1)!;
    }
    if (step.field_key === "promotion.landing_page_reference") {
      const landingInput = this.page.getByPlaceholder(step.target, { exact: true });
      if ((await landingInput.count()) === 1) {
        if (step.operation === "fill_text" && await landingInput.isVisible()) return landingInput;
        const pickerControl = landingInput.locator("xpath=following-sibling::*[contains(@class,'input__suffix')][1]");
        if ((await pickerControl.count()) === 1 && await pickerControl.isVisible()) return pickerControl;
      }
    }
    if (step.field_key === "promotion.comments_enabled") {
      const commentScope = this.page.getByText("单元评论", { exact: true }).last();
      const option = commentScope.locator("xpath=ancestor::*[contains(@class,'oc-row')][1]").getByText(step.target, { exact: true });
      if ((await option.count()) === 1 && await option.isVisible()) return option;
    }
    if (step.field_key === "promotion.smart_generation_enabled") {
      const checkbox = this.page.getByRole("checkbox", { name: "开启智能生成", exact: true });
      if ((await checkbox.count()) === 1 && await checkbox.isVisible()) return checkbox;
    }
    if (step.field_key === "project.aigc_dynamic_creative") {
      const control = this.page.locator("[data-auto-id='switch-btn'][data-e2e='createproject_aigcDynamic']:visible");
      if ((await control.count()) === 1) return control;
      throw new RunnerV3Error("page_drift", `${step.id}: AIGC switch is unavailable`);
    }
    if (step.target === "spinbutton") {
      if (step.field_key === "project.daily_budget" && (await this.page.getByRole("spinbutton").count()) === 0) {
        const reveal = await this.uniqueVisibleText("设置预算", step.id);
        await reveal.click();
        for (let attempt = 0; attempt < 20 && (await this.page.getByRole("spinbutton").count()) === 0; attempt += 1) {
          await this.page.waitForTimeout(200);
        }
      }
      if (step.field_key === "promotion.daily_budget" || step.field_key === "promotion.bid") {
        const promotionMoney = this.page.getByRole("spinbutton");
        for (let attempt = 0; attempt < 40 && (await promotionMoney.count()) < 2; attempt += 1) {
          await this.page.waitForTimeout(250);
        }
        if ((await promotionMoney.count()) === 2) {
          return promotionMoney.nth(step.field_key === "promotion.daily_budget" ? 0 : 1);
        }
      }
      if (step.field_key === "project.daily_budget" || step.field_key === "project.bid") {
        const projectMoney = this.page.getByRole("spinbutton");
        for (let attempt = 0; attempt < 40 && (await projectMoney.count()) < 2; attempt += 1) {
          await this.page.waitForTimeout(250);
        }
        if ((await projectMoney.count()) === 2) {
          // The calibrated sales-lead form renders daily budget before bid.
          return projectMoney.nth(step.field_key === "project.daily_budget" ? 0 : 1);
        }
      }
      const scoped = scope.first().locator("xpath=ancestor::*[.//*[@role='spinbutton'] or .//input][1]").getByRole("spinbutton");
      if ((await scoped.count()) === 1) return scoped;
      const all = this.page.getByRole("spinbutton");
      if ((await all.count()) === 1) return all;
      throw new RunnerV3Error("locator_not_unique", `${step.id}: spinbutton is not unique`);
    }
    const scopedInput = scope.first()
      .locator("xpath=ancestor::*[.//input][1]")
      .getByPlaceholder(step.target, { exact: true });
    if ((await scopedInput.count()) === 1 && await scopedInput.isVisible()) return scopedInput;
    const placeholder = this.page.getByPlaceholder(step.target, { exact: true });
    if ((await placeholder.count()) === 1 && await placeholder.isVisible()) return placeholder;
    const scopedText = scope.first()
      .locator(`xpath=ancestor::*[.//*[normalize-space(.)=${this.xpathLiteral(step.target)}]][1]`)
      .getByText(step.target, { exact: true });
    if ((await scopedText.count()) === 1 && await scopedText.isVisible()) return scopedText;
    const text = this.page.getByText(step.target, { exact: true });
    if ((await text.count()) === 1) return text;
    throw new RunnerV3Error("locator_not_unique", `${step.id}: target is not unique`);
  }

  async applyField(step: PlanStep) {
    if (!step.operation) throw new RunnerV3Error("invalid_plan", `${step.id}: operation is required`);
    if (step.field_key === "project.marketing_product_reference" && this.isReferenceSelectionSpec(step.value)) {
      const label = step.value.label;
      if (label) {
        const existingProduct = this.page.getByText(label, { exact: true });
        for (let index = 0; index < await existingProduct.count(); index += 1) {
          if (await existingProduct.nth(index).isVisible()) {
            const selectedCard = existingProduct.nth(index).locator("xpath=ancestor::*[contains(concat(' ',normalize-space(@class),' '),' oc-create-product-card ')][1]");
            if (step.value.object_id && ((await selectedCard.count()) !== 1 || !(await selectedCard.innerText()).includes(step.value.object_id))) continue;
            this.referenceReadbacks.set(step.id, {
              selection_kind: step.value.selection_kind,
              selected_count: 1,
              label,
              object_id: step.value.object_id,
              reused_existing_selection: true,
            });
            return;
          }
        }
      }
    }
    if (step.field_key === "promotion.delivery_identity") {
      const spec = this.isReferenceSelectionSpec(step.value) ? step.value : undefined;
      const label = spec?.label ?? (typeof step.value === "string" ? step.value : undefined);
      if (!spec && (label === "账户信息" || label === "账号信息")) {
        let accountInfo = this.page.locator("[data-e2e='createad_nativetype_0']:visible");
        for (let attempt = 0; attempt < 60 && (await accountInfo.count()) === 0; attempt += 1) {
          await this.page.waitForTimeout(250);
          accountInfo = this.page.locator("[data-e2e='createad_nativetype_0']:visible");
        }
        if ((await accountInfo.count()) !== 1) {
          throw new RunnerV3Error("locator_not_unique", `${step.id}: account identity option is not unique`);
        }
        if (!(await accountInfo.getAttribute("class"))?.includes("ovui-radio-item--checked")) {
          await accountInfo.click();
        }
        this.referenceReadbacks.set(step.id, { selection_kind: "text_option", selected_count: 1, label: "账户信息" });
        return;
      }
      if (label) {
        const selected = this.page.getByText(label, { exact: true });
        for (let attempt = 0; attempt < 40 && (await selected.count()) === 0; attempt += 1) {
          await this.page.waitForTimeout(250);
        }
        for (let index = 0; index < await selected.count(); index += 1) {
          if (await selected.nth(index).isVisible()) {
            this.referenceReadbacks.set(step.id, { selection_kind: spec?.selection_kind ?? "text_option", selected_count: 1, label });
            return;
          }
        }
      }
    }
    if (step.field_key === "promotion.base_materials") {
      const existingVideo = this.page.getByText(/视频\([1-9]\d*\/\d+\)/);
      if ((await existingVideo.count()) === 1 && await existingVideo.isVisible()) {
        this.referenceReadbacks.set(step.id, { selection_kind: "media_card", selected_count: 1, reused_existing_selection: true });
        return;
      }
    }
    if (step.field_key === "promotion.product_selling_points") {
      const values = Array.isArray(step.value) ? step.value.map(String) : [String(step.value)];
      const sellingPointScope = this.page.getByText("产品卖点", { exact: true }).first();
      const sellingPointRow = sellingPointScope.locator(
        "xpath=ancestor::div[contains(concat(' ',normalize-space(@class),' '),' oc-row ')][1]",
      );
      let allSelected = values.length > 0;
      for (const value of values) {
        const selected = sellingPointRow.getByText(value, { exact: true });
        if ((await selected.count()) < 1) allSelected = false;
      }
      if (allSelected) {
        this.referenceReadbacks.set(step.id, values);
        return;
      }
    }
    if (step.field_key === "promotion.product_image_references") {
      if (!this.isReferenceSelectionSpec(step.value) || !isStablePlatformImageSourceIdentity(step.value.image_src_identity ?? "")) {
        throw new RunnerV3Error("invalid_plan", `${step.id}: stable product image source identity is required`);
      }
      const productImageScope = this.page.getByText("产品主图", { exact: true }).first();
      const productImageRow = productImageScope.locator(
        "xpath=ancestor::div[contains(concat(' ',normalize-space(@class),' '),' oc-row ')][1]",
      );
      const selectedCount = productImageRow.getByText(/已添加：\s*[1-9]\d*\/\d+/);
      if ((await selectedCount.count()) === 1 && await selectedCount.isVisible()) {
        const matching = await this.imagesMatchingSourceIdentity(productImageRow.locator("img:visible"), step.value.image_src_identity!);
        if (matching.length !== 1) {
          throw new RunnerV3Error(matching.length > 1 ? "locator_not_unique" : "object_mismatch", `${step.id}: selected product image does not uniquely match the plan`);
        }
        this.referenceReadbacks.set(step.id, { selection_kind: "image_card", selected_count: 1, image_src_identity: step.value.image_src_identity, reused_existing_selection: true });
        return;
      }
    }
    if (step.field_key === "promotion.call_to_action") {
      const values = Array.isArray(step.value) ? step.value.map(String) : [];
      if (values.length < 1 || values.length > 10 || new Set(values).size !== values.length || values.some((value) => !value.trim())) {
        throw new RunnerV3Error("invalid_value", `${step.id}: call to action needs 1 to 10 unique values`);
      }
      let selected = await this.selectedCallToActions();
      for (let attempt = 0; attempt < 20 && selected.length === 0; attempt += 1) {
        await this.page.waitForTimeout(250);
        selected = await this.selectedCallToActions();
      }
      const matches = selected.length === values.length && values.every((value) => selected.includes(value));
      if (!matches) {
        const root = this.page.locator("[data-e2e='createad_actionText__createRecommendTag']:visible");
        if ((await root.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: call-to-action input is not unique`);
        for (const value of selected.filter((item) => !values.includes(item))) {
          const tags = root.locator(".oc-tag");
          const matching: Locator[] = [];
          for (let index = 0; index < await tags.count(); index += 1) {
            const tag = tags.nth(index);
            if ((await tag.locator(".oc-tag-text").innerText()).trim() === value) matching.push(tag);
          }
          if (matching.length !== 1) throw new RunnerV3Error("page_drift", `${step.id}: call-to-action tag is not unique`);
          const close = matching[0].locator(".ovui-tag__close");
          if ((await close.count()) !== 1) throw new RunnerV3Error("page_drift", `${step.id}: call-to-action tag cannot be removed`);
          await close.evaluate((element) => (element as HTMLElement).click());
          for (let attempt = 0; attempt < 20 && (await this.selectedCallToActions()).includes(value); attempt += 1) {
            await this.page.waitForTimeout(100);
          }
          if ((await this.selectedCallToActions()).includes(value)) {
            throw new RunnerV3Error("field_readback_mismatch", `${step.id}: call-to-action tag was not removed: ${value}`);
          }
        }
        for (const value of values) {
          if ((await this.selectedCallToActions()).includes(value)) continue;
          const input = root.locator("input:visible");
          if ((await input.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: call-to-action input is not unique`);
          for (let attempt = 0; attempt < 3 && !(await this.selectedCallToActions()).includes(value); attempt += 1) {
            await input.fill(value);
            await input.press("Enter");
            for (let check = 0; check < 10 && !(await this.selectedCallToActions()).includes(value); check += 1) {
              await this.page.waitForTimeout(200);
            }
          }
        }
      }
      let observed: string[] = [];
      for (let attempt = 0; attempt < 40; attempt += 1) {
        observed = await this.selectedCallToActions();
        if (observed.length === values.length && values.every((value) => observed.includes(value))) break;
        await this.page.waitForTimeout(250);
      }
      if (observed.length !== values.length || !values.every((value) => observed.includes(value))) {
        const root = this.page.locator("[data-e2e='createad_actionText__createRecommendTag']:visible");
        const input = root.locator("input:visible");
        const diagnostic = (await root.count()) === 1
          ? {
              text: (await root.innerText()).replace(/\s+/g, " ").trim(),
              input_value: (await input.count()) === 1 ? await input.inputValue() : undefined,
              input_disabled: (await input.count()) === 1 ? await input.isDisabled() : undefined,
            }
          : { root_count: await root.count() };
        throw new RunnerV3Error(
          "field_readback_mismatch",
          `${step.id}: call-to-action set does not match the plan; initial=${JSON.stringify(selected)}; observed=${JSON.stringify(observed)}; control=${JSON.stringify(diagnostic)}`,
        );
      }
      this.referenceReadbacks.set(step.id, observed);
      return;
    }
    if (step.field_key === "promotion.category") {
      const target = await this.targetLocator(step);
      await target.click();
      const popper = this.page.locator("[data-e2e='createad_yuntuCategory_popover_content']:visible");
      for (let attempt = 0; attempt < 40 && (await popper.count()) === 0; attempt += 1) {
        await this.page.waitForTimeout(250);
      }
      if ((await popper.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: category picker is not unique`);
      const value = String(step.value);
      const search = popper.locator("input[placeholder='请输入内容']");
      if ((await search.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: category search is not unique`);
      await search.fill(value);
      const result = popper.locator(".ovui-cascader-search-option").filter({ hasText: value });
      for (let attempt = 0; attempt < 40 && (await result.count()) === 0; attempt += 1) {
        await this.page.waitForTimeout(250);
      }
      if ((await result.count()) !== 1) throw new RunnerV3Error("async_load_timeout", `${step.id}: category search result did not load`);
      await result.click();
      return;
    }
    if (step.field_key === "project.search_targeting_expansion") {
      const value = String(step.value);
      const optionIndex = value === "启用" ? "1" : value === "不启用" ? "2" : undefined;
      if (!optionIndex) throw new RunnerV3Error("invalid_value", `${step.id}: unsupported targeting expansion value`);
      const option = this.page.locator(`[data-e2e='createproject_audienceextend_${optionIndex}']:visible`);
      if ((await option.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: targeting expansion option is not unique`);
      if (!(await option.getAttribute("class"))?.includes("ovui-radio-item--checked")) await option.click();
      return;
    }
    if (step.field_key === "promotion.brand_reference" && this.isReferenceSelectionSpec(step.value)) {
      const label = step.value.label;
      if (!label) throw new RunnerV3Error("invalid_value", `${step.id}: brand label is required`);
      const target = await this.targetLocator(step);
      await target.click();
      let option = this.page.getByText(label, { exact: true });
      let visibleOption: Locator | undefined;
      for (let index = 0; index < await option.count(); index += 1) {
        if (await option.nth(index).isVisible()) visibleOption = option.nth(index);
      }
      if (!visibleOption) {
        const custom = this.page.getByRole("button", { name: "自定义品牌名称", exact: true });
        if ((await custom.count()) !== 1 || !(await custom.isVisible())) {
          throw new RunnerV3Error("async_load_timeout", `${step.id}: brand option did not load`);
        }
        await custom.click();
        const confirm = this.page.getByRole("button", { name: "确定", exact: true });
        const confirmScope = confirm.locator("xpath=ancestor::*[.//input][1]");
        const customInput = confirmScope.getByPlaceholder("请输入", { exact: true });
        if ((await customInput.count()) !== 1) {
          throw new RunnerV3Error("locator_not_unique", `${step.id}: custom brand input is not unique`);
        }
        await customInput.fill(label);
        await confirm.click();
        option = this.page.getByText(label, { exact: true });
        for (let attempt = 0; attempt < 20 && !visibleOption; attempt += 1) {
          for (let index = 0; index < await option.count(); index += 1) {
            if (await option.nth(index).isVisible()) visibleOption = option.nth(index);
          }
          if (!visibleOption) await this.page.waitForTimeout(100);
        }
      }
      if (!visibleOption) throw new RunnerV3Error("async_load_timeout", `${step.id}: brand option did not become visible`);
      await visibleOption.click();
      this.referenceReadbacks.set(step.id, {
        selection_kind: step.value.selection_kind,
        selected_count: 1,
        label,
      });
      return;
    }
    if (step.operation === "choose_exact_visible_option") {
      const value = String(step.value);
      await this.confirmKnownFieldTransition(step, false);
      const inlineOption = await this.projectInlineOption(step, value);
      if (inlineOption) {
        const optionClass = (await inlineOption.getAttribute("class")) ?? "";
        if (
          optionClass.includes("ovui-radio-item--checked") ||
          (step.field_key === "project.marketing_purpose" && optionClass.split(/\s+/).includes("active"))
        ) return;
        await inlineOption.click();
        await this.confirmKnownFieldTransition(step, true);
        return;
      }
      if (step.target !== value) {
        const target = await this.targetLocator(step);
        if (!(await target.isEnabled())) {
          const observed = await target.inputValue().catch(async () => (await target.textContent()) ?? "");
          if (observed.trim() === value) return;
          throw new RunnerV3Error("immutable_field_mismatch", `${step.id}: locked field does not match the requested value`);
        }
        await target.click();
        const option = await this.uniqueVisibleText(value, step.id);
        await option.click();
      } else {
        const option = await this.targetLocator(step);
        await option.click();
      }
      await this.confirmKnownFieldTransition(step, true);
      return;
    }

    const target = await this.targetLocator(step);
    if (step.operation === "fill_text" || step.operation === "fill_money" || step.operation === "fill_decimal") {
      await target.fill(String(step.value));
    } else if (step.operation === "toggle") {
      if (typeof step.value !== "boolean") throw new RunnerV3Error("invalid_value", `${step.id}: toggle needs a boolean`);
      await this.setCheckbox(target, step.value, step.id);
    } else if (step.operation === "open_reference_picker") {
      const initialProductRequest = step.field_key === "project.marketing_product_reference"
        ? this.waitForProductListRequest(undefined, 2_000)
        : undefined;
      await target.click();
      if (initialProductRequest) {
        const request = await initialProductRequest;
        if (request) await this.finishProductListRequest(request, step.id);
        else await this.waitForStableInitialProductList(step.id);
      }
      if (this.isReferenceSelectionSpec(step.value)) {
        this.referenceReadbacks.set(step.id, await this.selectReference(step, step.value));
      } else {
        const values = Array.isArray(step.value) ? step.value : [step.value];
        for (const value of values) {
          const option = await this.uniqueVisibleText(String(value), step.id);
          await option.click();
        }
        this.referenceReadbacks.set(step.id, { selection_kind: "text_option", selected_count: values.length, values });
      }
    } else if (step.operation === "configure_object") {
      await this.configureObject(step, target);
    } else {
      throw new RunnerV3Error("invalid_plan", `${step.id}: unsupported operation ${step.operation}`);
    }
  }

  private isReferenceSelectionSpec(value: unknown): value is ReferenceSelectionSpec {
    if (!value || Array.isArray(value) || typeof value !== "object") return false;
    return ["text_option", "async_row", "media_card", "image_card"].includes(
      String((value as Record<string, unknown>).selection_kind),
    );
  }

  private async uniqueVisibleText(value: string | RegExp, stepId: string, root: Page | Locator = this.page) {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      const options = root.getByText(value, typeof value === "string" ? { exact: true } : undefined);
      const visible: Locator[] = [];
      for (let index = 0; index < await options.count(); index += 1) {
        const option = options.nth(index);
        if (await option.isVisible()) visible.push(option);
      }
      if (visible.length === 1) return visible[0];
      if (visible.length > 1) throw new RunnerV3Error("locator_not_unique", `${stepId}: visible reference option is not unique`);
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: reference option did not load`);
  }

  private async stableVisibleCount(locator: Locator, minimum: number, stepId: string) {
    let prior = -1;
    let stable = 0;
    for (let attempt = 0; attempt < 40; attempt += 1) {
      let count = 0;
      for (let index = 0; index < await locator.count(); index += 1) {
        if (await locator.nth(index).isVisible()) count += 1;
      }
      stable = count === prior ? stable + 1 : 0;
      if (count >= minimum && stable >= 2) return count;
      prior = count;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: visible reference count did not become stable`);
  }

  private async pickerRoot() {
    const dialogs = this.page.locator("[role='dialog']:visible,.ovui-modal:visible,[class*='modal']:visible,.ovui-drawer:visible");
    for (let attempt = 0; attempt < 40 && (await dialogs.count()) === 0; attempt += 1) {
      await this.page.waitForTimeout(250);
    }
    return (await dialogs.count()) > 0 ? dialogs.last() : this.page.locator("body");
  }

  private async confirmKnownFieldTransition(step: PlanStep, waitForAppearance: boolean) {
    if (step.field_key !== "project.marketing_purpose") return;
    const message = "切换营销目的将会清空您已填写的所有内容，是否继续切换？";
    let modal = this.page.locator(".ovui-modal__wrap:visible").filter({ hasText: message });
    for (let attempt = 0; waitForAppearance && attempt < 20 && (await modal.count()) === 0; attempt += 1) {
      await this.page.waitForTimeout(100);
      modal = this.page.locator(".ovui-modal__wrap:visible").filter({ hasText: message });
    }
    if ((await modal.count()) === 0) return;
    if ((await modal.count()) !== 1) {
      throw new RunnerV3Error("locator_not_unique", `${step.id}: marketing-purpose confirmation is not unique`);
    }
    const confirm = modal.getByRole("button", { name: "确定", exact: true });
    if ((await confirm.count()) !== 1 || !(await confirm.isVisible())) {
      throw new RunnerV3Error("page_drift", `${step.id}: marketing-purpose confirmation action is unavailable`);
    }
    await confirm.click();
    await modal.waitFor({ state: "hidden", timeout: 5_000 });
  }

  private async projectInlineOption(step: PlanStep, value: string): Promise<Locator | undefined> {
    let option: Locator | undefined;
    if (step.field_key === "project.marketing_purpose") {
      option = this.page.locator("[data-e2e='createproject_landingtype__ocSwitchCard']:visible").filter({
        has: this.page.getByText(value, { exact: true }),
      });
    } else if (step.field_key === "project.lead_capture_mode") {
      const dataE2E = value === "智能优选"
        ? "createproject_assetType_multioption_1"
        : value === "自定义" ? "createproject_assetType_multioption_0" : undefined;
      if (!dataE2E) throw new RunnerV3Error("invalid_value", `${step.id}: unsupported lead capture mode`);
      option = this.page.locator(`[data-e2e='${dataE2E}']:visible`);
    } else if (step.field_key === "project.delivery_mode") {
      const expectedSuffix = value === "手动投放" ? "_1" : value === "自动投放(UBMax)" ? "_3" : undefined;
      if (!expectedSuffix) throw new RunnerV3Error("invalid_value", `${step.id}: unsupported delivery mode`);
      // Hidden controls from an old marketing branch must not match.
      option = this.page.locator(`[data-e2e='createproject_deliverymode${expectedSuffix}']:visible`);
    }
    if (!option) return undefined;
    for (let attempt = 0; attempt < 40 && (await option.count()) === 0; attempt += 1) {
      await this.page.waitForTimeout(250);
    }
    if ((await option.count()) !== 1) {
      throw new RunnerV3Error("locator_not_unique", `${step.id}: inline option is not unique`);
    }
    return option;
  }

  private waitForProductListRequest(query: string | undefined, timeout: number) {
    return this.page.waitForRequest((request) => {
      if (
        !request.url().includes("/superior/api/agw/ad/recommend_product_list") &&
        !request.url().includes("/superior/api/v2/ad/product/clue_product_list")
      ) return false;
      if (!query) return true;
      return request.url().includes(encodeURIComponent(query)) || (request.postData() ?? "").includes(query);
    }, { timeout }).catch(() => undefined);
  }

  private async finishProductListRequest(request: Request, stepId: string) {
    const response = await request.response();
    if (!response) throw new RunnerV3Error("async_load_timeout", `${stepId}: product list request has no response`);
    await response.finished();
    if (!response.ok()) throw new RunnerV3Error("async_load_timeout", `${stepId}: product list request failed`);
  }

  private async waitForStableInitialProductList(stepId: string) {
    const cards = this.page.locator("[data-e2e='createproject_productselectdrawer']:visible [data-auto-id='create-product-card']:visible");
    let prior = "";
    let stable = 0;
    for (let attempt = 0; attempt < 40; attempt += 1) {
      const signature = await cards.allInnerTexts().then((values) => values.join("\u0000"));
      stable = signature && signature === prior ? stable + 1 : 0;
      if (stable >= 4) return;
      prior = signature;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: initial product list did not become stable`);
  }

  private async waitForStableProductCard(root: Locator, label: string, objectId: string, stepId: string) {
    let prior = "";
    let stable = 0;
    for (let attempt = 0; attempt < 60; attempt += 1) {
      const labels = root.getByText(label, { exact: true });
      const matchingCards: Locator[] = [];
      for (let index = 0; index < await labels.count(); index += 1) {
        const candidate = labels.nth(index);
        if (!await candidate.isVisible()) continue;
        const card = candidate.locator("xpath=ancestor::*[contains(concat(' ',normalize-space(@class),' '),' oc-create-product-card ')][1]");
        if ((await card.count()) === 1 && (await card.innerText()).includes(objectId)) matchingCards.push(card);
      }
      if (matchingCards.length > 1) throw new RunnerV3Error("locator_not_unique", `${stepId}: searched product card is not unique`);
      const signature = matchingCards.length === 1 ? await matchingCards[0].innerText() : "";
      stable = signature && signature === prior ? stable + 1 : 0;
      if (matchingCards.length === 1 && stable >= 4) return matchingCards[0];
      prior = signature;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: searched product result did not become stable`);
  }

  private async waitForStableMaterialCard(root: Locator, label: string, query: string, stepId: string) {
    const search = root.locator("input[placeholder='可搜索视频名称或ID']:visible");
    if ((await search.count()) !== 1) {
      throw new RunnerV3Error("locator_not_unique", `${stepId}: material search input is not unique`);
    }
    // Let the initial account-wide list finish before filtering. Otherwise a
    // late initial response can overwrite the filtered result.
    await this.waitForStableMaterialInventory(root, stepId);
    await search.fill(query);
    await search.press("Enter");
    let prior = "";
    let stable = 0;
    for (let attempt = 0; attempt < 60; attempt += 1) {
      const labels = root.getByText(label, { exact: true });
      const cards: Locator[] = [];
      for (let index = 0; index < await labels.count(); index += 1) {
        const candidate = labels.nth(index);
        if (!await candidate.isVisible()) continue;
        const card = candidate.locator("xpath=ancestor::*[contains(concat(' ',normalize-space(@class),' '),' create-material-list-card-item ')][1]");
        if ((await card.count()) === 1) cards.push(card);
      }
      if (cards.length > 1) throw new RunnerV3Error("locator_not_unique", `${stepId}: searched material card is not unique`);
      const signature = cards.length === 1 && await search.inputValue() === query
        ? await cards[0].innerText()
        : "";
      stable = signature && signature === prior ? stable + 1 : 0;
      if (cards.length === 1 && stable >= 4) return cards[0];
      prior = signature;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: searched material result did not become stable`);
  }

  private async waitForStableMaterialInventory(root: Locator, stepId: string) {
    const cards = root.locator(".create-material-list-card-item:visible");
    let prior = "";
    let stable = 0;
    for (let attempt = 0; attempt < 80; attempt += 1) {
      const texts = await cards.allInnerTexts();
      const signature = texts.join("\u0000");
      stable = signature && signature === prior ? stable + 1 : 0;
      if (texts.length > 0 && stable >= 8) return;
      prior = signature;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("async_load_timeout", `${stepId}: initial material inventory did not become stable`);
  }

  private async confirmPicker(root: Locator, spec: ReferenceSelectionSpec, stepId: string) {
    const buttonName = spec.confirm_button ?? "确定";
    const local = root.getByRole("button", { name: buttonName, exact: true });
    if ((await local.count()) === 1 && await local.isVisible()) {
      await local.click();
      return;
    }
    const global = this.page.getByRole("button", { name: buttonName, exact: true });
    const visible: Locator[] = [];
    for (let index = 0; index < await global.count(); index += 1) {
      if (await global.nth(index).isVisible()) visible.push(global.nth(index));
    }
    if (visible.length !== 1) throw new RunnerV3Error("locator_not_unique", `${stepId}: picker confirmation is not unique`);
    await visible[0].click();
  }

  private async checkboxState(checkbox: Locator, stepId: string) {
    const state = await checkbox.evaluate((element) => {
      const input = element instanceof HTMLInputElement && element.type === "checkbox"
        ? element
        : element.querySelector<HTMLInputElement>("input[type='checkbox']");
      if (input) return input.checked;
      const ariaChecked = element.getAttribute("aria-checked");
      if (ariaChecked === "true" || ariaChecked === "false") return ariaChecked === "true";
      const visualSwitch = element.matches(".ovui-switch") ? element : element.querySelector(".ovui-switch");
      if (visualSwitch) return visualSwitch.classList.contains("ovui-switch--checked");
      return null;
    });
    if (state === null) throw new RunnerV3Error("page_drift", `${stepId}: toggle state is unavailable`);
    return state;
  }

  private async setCheckbox(checkbox: Locator, checked: boolean, stepId: string) {
    if (await this.checkboxState(checkbox, stepId) === checked) return;
    const visualControl = checkbox.locator("xpath=following-sibling::*[contains(@class,'checkbox__inner')][1]");
    if ((await visualControl.count()) === 1 && await visualControl.isVisible()) {
      await visualControl.click();
    } else if (await checkbox.evaluate((element) => element instanceof HTMLInputElement && element.type === "checkbox")) {
      await checkbox.setChecked(checked, { force: true });
    } else {
      await checkbox.click();
    }
    if (await this.checkboxState(checkbox, stepId) !== checked) {
      throw new RunnerV3Error("reference_not_selected", `${stepId}: checkbox state did not change`);
    }
  }

  private async imageSourceIdentity(image: Locator) {
    const source = await image.evaluate((element) => {
      const imageElement = element as HTMLImageElement;
      return imageElement.currentSrc || imageElement.src || imageElement.getAttribute("src") || "";
    });
    return canonicalImageSourceIdentity(source);
  }

  private async imagesMatchingSourceIdentity(images: Locator, expected: string) {
    const identity = canonicalImageSourceIdentity(expected);
    const matches: Locator[] = [];
    for (let index = 0; index < await images.count(); index += 1) {
      const image = images.nth(index);
      if (await this.imageSourceIdentity(image) === identity) matches.push(image);
    }
    return matches;
  }

  private async waitForStableProductImage(root: Locator, expected: string, stepId: string) {
    const expectedIdentity = canonicalImageSourceIdentity(expected);
    if (!expectedIdentity) throw new RunnerV3Error("invalid_value", `${stepId}: product image source identity is invalid`);
    let prior = "";
    let stable = 0;
    let visibleCount = 0;
    for (let attempt = 0; attempt < 60; attempt += 1) {
      const images = root.locator("img:visible");
      const identities: string[] = [];
      for (let index = 0; index < await images.count(); index += 1) {
        identities.push(await this.imageSourceIdentity(images.nth(index)));
      }
      visibleCount = identities.length;
      const signature = identities.join("\u0000");
      stable = signature && signature === prior ? stable + 1 : 0;
      if (stable >= 8) {
        const matchedIndexes = identities.flatMap((identity, index) => identity === expectedIdentity ? [index] : []);
        if (matchedIndexes.length > 1) throw new RunnerV3Error("locator_not_unique", `${stepId}: product image source matched multiple images`);
        if (matchedIndexes.length === 1) return { image: images.nth(matchedIndexes[0]), visibleCount, identity: expectedIdentity };
      }
      prior = signature;
      await this.page.waitForTimeout(250);
    }
    throw new RunnerV3Error("object_mismatch", `${stepId}: product image source was not found in the stable picker inventory`);
  }

  private async selectReference(step: PlanStep, spec: ReferenceSelectionSpec) {
    const root = await this.pickerRoot();
    const projectProduct = step.field_key === "project.marketing_product_reference";
    if (projectProduct && spec.object_id) {
      const search = this.page.getByPlaceholder("请输入商品名称或ID", { exact: true });
      if ((await search.count()) !== 1 || !(await search.isVisible())) {
        throw new RunnerV3Error("page_drift", `${step.id}: product search input is unavailable`);
      }
      const filteredRequest = this.waitForProductListRequest(spec.object_id, 5_000);
      await search.fill(spec.object_id);
      await search.press("Enter");
      const request = await filteredRequest;
      if (request) await this.finishProductListRequest(request, step.id);
      if (!spec.label) throw new RunnerV3Error("invalid_value", `${step.id}: product label is required`);
      const productCard = await this.waitForStableProductCard(root, spec.label, spec.object_id, step.id);
      const checkbox = productCard.locator("input[type='checkbox']");
      if ((await checkbox.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: product checkbox is not unique`);
      // The ecommerce picker shows 1/1. The sales-lead drawer shows 1/10,
      // and some revisions update that counter after the confirm action. The
      // checked control is the stable pre-confirm selection evidence.
      await this.setCheckbox(checkbox, true, step.id);
      await this.confirmPicker(root, spec, step.id);
      return {
        selection_kind: spec.selection_kind,
        selected_count: 1,
        label: spec.label,
        object_id: spec.object_id,
        element_verified: "searched_product_card_and_unique_product_id",
      };
    }
    if (step.field_key === "promotion.base_materials" && spec.object_id && spec.label) {
      const card = await this.waitForStableMaterialCard(root, spec.label, spec.object_id, step.id);
      const checkbox = card.locator("input[type='checkbox']");
      if ((await checkbox.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: material checkbox is not unique`);
      await this.setCheckbox(checkbox, true, step.id);
      await this.stableVisibleCount(root.getByText(/已选择\s*1\s*\/\s*30/), 1, step.id);
      await this.confirmPicker(root, spec, step.id);
      return {
        selection_kind: spec.selection_kind,
        selected_count: 1,
        label: spec.label,
        object_id: spec.object_id,
        element_verified: "searched_material_card_and_object_id",
      };
    }
    // Connector catalog totals describe the synchronized Cookies subset.
    // The project picker can contain a larger account-wide product catalog.
    if (spec.expected_total !== undefined && !projectProduct && step.field_key !== "promotion.product_image_references") {
      const total = this.page.getByText(new RegExp(`共\\s*${spec.expected_total}\\s*条`));
      await this.stableVisibleCount(total, 1, step.id);
    }
    if (spec.selection_kind === "text_option" || spec.selection_kind === "async_row") {
      if (!spec.label && !spec.object_id) {
        throw new RunnerV3Error("invalid_value", `${step.id}: reference label or object_id is required`);
      }
      if (spec.object_id) {
        const idText = await this.uniqueVisibleText(new RegExp(`ID[:：]\\s*${spec.object_id}`), step.id, root);
        const cardRow = idText.locator("xpath=ancestor::*[contains(@class,'create-material-list-card-item')][1]");
        if ((await cardRow.count()) === 1) {
          if (spec.label && !(await cardRow.innerText()).includes(spec.label)) {
            throw new RunnerV3Error("object_mismatch", `${step.id}: reference ID does not match the expected label`);
          }
          await cardRow.click();
          const selectedCount = root.getByText(/已选择\s*1\s*\/\s*1/);
          await this.stableVisibleCount(selectedCount, 1, step.id);
          await this.confirmPicker(root, spec, step.id);
          return {
            selection_kind: spec.selection_kind,
            selected_count: 1,
            expected_total: spec.expected_total,
            label: spec.label,
            object_id: spec.object_id,
            element_verified: "card_and_selected_count",
          };
        }
        const row = idText.locator("xpath=ancestor::*[.//*[@role='checkbox'] or .//input[@type='checkbox']][1]");
        if ((await row.count()) !== 1) throw new RunnerV3Error("page_drift", `${step.id}: reference row has no unique checkbox scope`);
        if (spec.label && !(await row.innerText()).includes(spec.label)) {
          throw new RunnerV3Error("object_mismatch", `${step.id}: reference ID does not match the expected label`);
        }
        const checkbox = row.locator("input[type='checkbox'],[role='checkbox']");
        if ((await checkbox.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: reference row checkbox is not unique`);
        await this.setCheckbox(checkbox, true, step.id);
      } else {
        const option = await this.uniqueVisibleText(spec.label!, step.id, root);
        await option.click();
      }
      await this.confirmPicker(root, spec, step.id);
      return {
        selection_kind: spec.selection_kind,
        selected_count: 1,
        expected_total: spec.expected_total,
        label: spec.label,
        object_id: spec.object_id,
      };
    }

    if (step.field_key === "promotion.product_image_references" && spec.selection_kind === "image_card") {
      if (!isStablePlatformImageSourceIdentity(spec.image_src_identity ?? "")) throw new RunnerV3Error("invalid_plan", `${step.id}: stable product image source identity is required`);
      const matched = await this.waitForStableProductImage(root, spec.image_src_identity!, step.id);
      if (matched.visibleCount < (spec.minimum_visible ?? 1)) throw new RunnerV3Error("async_load_timeout", `${step.id}: product image inventory did not load`);
      await matched.image.click();
      await this.stableVisibleCount(root.getByText(/已选择\s*1\s*\/\s*10/), 1, step.id);
      // Ocean Engine renders the selected image twice after a click: once in
      // the selected material card and once in the submit-bar preview. Verify
      // only the selected card, because the preview is not a second choice.
      const selectedCards = root.locator(
        ".oc-create-material-card-content.oc-create-material-card-selected:visible",
      );
      if ((await selectedCards.count()) !== 1) {
        throw new RunnerV3Error("field_readback_mismatch", `${step.id}: selected product image card is not unique`);
      }
      const verified = await this.imagesMatchingSourceIdentity(selectedCards.locator("img:visible"), matched.identity);
      if (verified.length !== 1) throw new RunnerV3Error("field_readback_mismatch", `${step.id}: selected product image identity changed after click`);
      await this.confirmPicker(root, spec, step.id);
      return {
        selection_kind: spec.selection_kind,
        selected_count: 1,
        visible_count: matched.visibleCount,
        image_src_identity: matched.identity,
        element_verified: "stable_absolute_img_src_and_selected_count",
      };
    }

    let cards = root.locator(".oc-create-material-card-content:visible");
    if (spec.selection_kind === "image_card") cards = cards.filter({ has: root.locator("img") });
    const visibleCount = await this.stableVisibleCount(cards, spec.minimum_visible ?? 1, step.id);
    let card: Locator;
    if (spec.label) {
      const labelled = cards.filter({ hasText: spec.label });
      if ((await labelled.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: material card is not unique`);
      card = labelled;
    } else {
      const index = spec.index ?? 0;
      if (index < 0 || index >= visibleCount) throw new RunnerV3Error("invalid_value", `${step.id}: material card index is out of range`);
      card = cards.nth(index);
    }
    if (spec.selection_kind === "image_card" && (await card.locator("img").count()) < 1) {
      throw new RunnerV3Error("page_drift", `${step.id}: selected image card has no img element`);
    }
    const checkbox = card.locator("input[type='checkbox'],[role='checkbox']");
    if ((await checkbox.count()) > 0) await this.setCheckbox(checkbox.first(), true, step.id);
    else await card.click();
    const checked = card.locator("input[type='checkbox']:checked,[role='checkbox'][aria-checked='true']");
    if ((await checked.count()) < 1 && !(await card.getAttribute("class"))?.includes("select")) {
      throw new RunnerV3Error("reference_not_selected", `${step.id}: material card did not report a selected state`);
    }
    await this.confirmPicker(root, spec, step.id);
    return {
      selection_kind: spec.selection_kind,
      selected_count: 1,
      visible_count: visibleCount,
      label: spec.label,
      index: spec.index ?? 0,
      element_verified: spec.selection_kind === "image_card" ? "img_and_checkbox" : "card_and_checkbox",
    };
  }

  private async configureObject(step: PlanStep, target: Locator) {
    if (step.field_key === "project.schedule") {
      const value = step.value as { start?: unknown; end?: unknown };
      if (!value || typeof value !== "object" || !value.start || !value.end) {
        throw new RunnerV3Error("invalid_value", `${step.id}: schedule needs start and end dates`);
      }
      await target.click();
      const start = this.page.getByPlaceholder("请选择开始日期", { exact: true });
      const end = this.page.getByPlaceholder("请选择结束日期", { exact: true });
      for (let attempt = 0; attempt < 40 && ((await start.count()) !== 1 || (await end.count()) !== 1); attempt += 1) {
        await this.page.waitForTimeout(250);
      }
      if ((await start.count()) !== 1 || (await end.count()) !== 1) {
        throw new RunnerV3Error("locator_not_unique", `${step.id}: schedule inputs are not unique`);
      }
      if (!(await start.isEnabled()) || !(await end.isEnabled())) {
        const observedStart = await start.inputValue();
        const observedEnd = await end.inputValue();
        if (observedStart === String(value.start) && observedEnd === String(value.end)) return;
        throw new RunnerV3Error("immutable_field_mismatch", `${step.id}: locked schedule does not match the requested value`);
      }
      await start.fill(String(value.start));
      await end.fill(String(value.end));
      return;
    }
    if (step.field_key === "project.placement_media") {
      if (!Array.isArray(step.value)) throw new RunnerV3Error("invalid_value", `${step.id}: placement media needs an array`);
      const selected = new Set(step.value.map(String));
      for (const name of ["今日头条", "西瓜视频", "抖音", "番茄系媒体", "穿山甲"]) {
        const label = this.page.getByText(name, { exact: true });
        if ((await label.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: media label ${name} is not unique`);
        const checkbox = label.locator("xpath=preceding-sibling::*[@role='checkbox'][1] | preceding-sibling::input[@type='checkbox'][1]");
        if ((await checkbox.count()) !== 1) throw new RunnerV3Error("page_drift", `${step.id}: media checkbox ${name} is missing`);
        await this.setCheckbox(checkbox, selected.has(name), step.id);
      }
      return;
    }
    if (step.field_key === "promotion.copy_materials" || step.field_key === "promotion.product_selling_points") {
      const values = Array.isArray(step.value) ? step.value.map(String) : [String(step.value)];
      if (step.field_key === "promotion.copy_materials") {
        await target.fill(values.join("\n"));
      } else {
        for (const value of values) {
          await target.fill(value);
          await target.press("Enter");
        }
        this.referenceReadbacks.set(step.id, values);
      }
      return;
    }
    throw new RunnerV3Error("operator_required", `${step.id}: complex object configuration needs a field-specific adapter`);
  }

  async readField(step: PlanStep) {
    if (this.referenceReadbacks.has(step.id)) {
      return this.referenceReadbacks.get(step.id);
    }
    if (step.operation === "toggle") {
      return this.checkboxState(await this.targetLocator(step), step.id);
    }
    if (step.operation === "choose_exact_visible_option" || step.operation === "open_reference_picker") {
      return step.value;
    }
    const target = await this.targetLocator(step);
    const tagName = String(await target.evaluate((element) => element.tagName)).toLowerCase();
    if (tagName === "input" || tagName === "textarea") return target.inputValue();
    return (await target.innerText()).trim();
  }

  async clickFinal(step: PlanStep) {
    await this.assertFinalReady(step);
    const button = this.page.getByRole("button", { name: step.target!, exact: true });
    this.preSubmitUrl = this.page.url();
    this.platformWriteRequestObserved = false;
    this.platformWriteResponseStatus = undefined;
    this.submittedExternalAction = undefined;
    this.page.on("request", (request) => {
      if (request.method() === "POST" && /\/superior\/api\/v2\/(?:project\/create|promotion\/create_promotion)(?:\?|$)/.test(request.url())) {
        this.platformWriteRequestObserved = true;
        if (/\/superior\/api\/v2\/project\/create(?:\?|$)/.test(request.url())) {
          try {
            const payload = request.postDataJSON() as Record<string, unknown>;
            const value = payload.external_action;
            if (typeof value === "string" || typeof value === "number") this.submittedExternalAction = String(value);
          } catch {
            // The write was still observed. Field reconciliation reports not_checked.
          }
        }
      }
    });
    this.page.on("response", (response) => {
      const request = response.request();
      if (request.method() === "POST" && /\/superior\/api\/v2\/(?:project\/create|promotion\/create_promotion)(?:\?|$)/.test(response.url())) {
        this.platformWriteRequestObserved = true;
        this.platformWriteResponseStatus = response.status();
      }
    });
    await button.click({ noWaitAfter: true });
  }

  async readMoneyConstraint(step: PlanStep) {
    const target = await this.targetLocator(step);
    const attributeValues = await Promise.all([
      target.getAttribute("min"),
      target.getAttribute("max"),
      target.getAttribute("aria-valuemin"),
      target.getAttribute("aria-valuemax"),
    ]);
    const minimumAttribute = attributeValues[0] ?? attributeValues[2];
    const maximumAttribute = attributeValues[1] ?? attributeValues[3];
    const attributeMinimum = minimumAttribute === null ? Number.NaN : Number(minimumAttribute);
    const attributeMaximum = maximumAttribute === null ? Number.NaN : Number(maximumAttribute);
    if (Number.isFinite(attributeMinimum) && Number.isFinite(attributeMaximum) && attributeMaximum >= attributeMinimum) {
      return {
        minimum_minor: Math.round(attributeMinimum * 100),
        maximum_minor: Math.round(attributeMaximum * 100),
      };
    }
    await target.blur();
    for (let attempt = 0; attempt < 10; attempt += 1) {
      const textValues = [
        await target.getAttribute("placeholder") ?? "",
        await target.getAttribute("aria-label") ?? "",
      ];
      for (const depth of [1, 2, 3]) {
        const ancestor = target.locator(`xpath=ancestor::*[${depth}]`);
        if ((await ancestor.count()) === 1 && await ancestor.isVisible()) textValues.push(await ancestor.innerText());
      }
      const parsed = parseOceanEngineMoneyConstraint(textValues);
      if (parsed) return parsed;
      await this.page.waitForTimeout(100);
    }
    return undefined;
  }

  async assertFinalReady(step: PlanStep) {
    if (!step.target) throw new RunnerV3Error("invalid_plan", `${step.id}: final target is required`);
    const button = this.page.getByRole("button", { name: step.target, exact: true });
    if ((await button.count()) !== 1) throw new RunnerV3Error("locator_not_unique", `${step.id}: final button is not unique`);
    if (!(await button.isEnabled())) throw new RunnerV3Error("submit_disabled", `${step.id}: final button is disabled`);
  }

  async observeSubmit(_plan: OceanEngineFormPlan): Promise<SubmitObservation> {
    for (let attempt = 0; attempt < 24; attempt += 1) {
      const success = this.page.getByText(/(?:保存|创建).{0,8}成功/);
      for (let index = 0; index < await success.count(); index += 1) {
        if (await success.nth(index).isVisible()) return this.submitObservation("success");
      }
      const errors = this.page.locator(".ovui-form-item-error,[class*='form-item-error'],[class*='FormItemError']");
      const messages: string[] = [];
      for (let index = 0; index < await errors.count(); index += 1) {
        const error = errors.nth(index);
        if (await error.isVisible()) messages.push((await error.innerText()).trim());
      }
      if (messages.some(Boolean)) return this.submitObservation("validation_error", messages.filter(Boolean).join("; "));
      const validationSummary = this.page.getByText("有些项目填写错误，请修改后再提交", { exact: true });
      if ((await validationSummary.count()) > 0 && await validationSummary.last().isVisible()) {
        return this.submitObservation("validation_error", "the platform reported form validation errors");
      }
      if (this.page.url() !== this.preSubmitUrl) return this.submitObservation("success");
      await this.page.waitForTimeout(250);
    }
    return this.submitObservation("result_unknown", "no success, validation error, or navigation was observed after one click");
  }

  private submitObservation(outcome: SubmitObservation["outcome"], errorMessage?: string): SubmitObservation {
    return {
      outcome,
      ...(errorMessage ? { error_message: errorMessage } : {}),
      platform_write_request_observed: this.platformWriteRequestObserved,
      ...(this.platformWriteResponseStatus !== undefined
        ? { platform_write_response_status: this.platformWriteResponseStatus }
        : {}),
    };
  }

  async reconcileSubmit(plan: OceanEngineFormPlan, observation: SubmitObservation): Promise<ReconciliationResult> {
    if (plan.plan_kind.endsWith("_edit")) {
      return plan.object_reference
        ? { status: "matched", created_object_id: plan.object_reference }
        : { status: "not_applicable" };
    }
    if (observation.created_object_id) return { status: "matched", created_object_id: observation.created_object_id };
    const queryKey = plan.plan_kind === "project_create" ? "project_id" : "promotion_id";
    let queryId: string | null = null;
    for (let attempt = 0; attempt < 40 && !queryId; attempt += 1) {
      queryId = new URL(this.page.url()).searchParams.get(queryKey);
      if (!queryId) await this.page.waitForTimeout(250);
    }
    if (queryId) return {
      status: "matched",
      created_object_id: queryId,
      ...(plan.plan_kind === "project_create" ? { field_reconciliation: this.reconcileProjectSubmission(plan) } : {}),
    };

    const nameKey = plan.plan_kind === "project_create" ? "project.project_name" : "promotion.promotion_name";
    const expectedName = plan.steps.find((step) => step.field_key === nameKey)?.value;
    if (typeof expectedName !== "string" || !expectedName) return { status: "not_applicable" };
    const placeholder = plan.plan_kind === "project_create"
      ? "输入项目ID或名称后回车搜索"
      : "输入单元ID或名称后回车搜索";
    let search = this.page.getByPlaceholder(placeholder, { exact: true });
    for (let attempt = 0; attempt < 40 && (await search.count()) === 0; attempt += 1) {
      await this.page.waitForTimeout(250);
      search = this.page.getByPlaceholder(placeholder, { exact: true });
    }
    if (plan.plan_kind === "promotion_create" && (await search.count()) === 0) {
      const promotionTabs = this.page.getByText("单元", { exact: true });
      for (let index = 0; index < await promotionTabs.count(); index += 1) {
        const promotionTab = promotionTabs.nth(index);
        if (await promotionTab.isVisible()) {
          await promotionTab.click();
          break;
        }
      }
      for (let attempt = 0; attempt < 40 && (await search.count()) === 0; attempt += 1) {
        await this.page.waitForTimeout(250);
        search = this.page.getByPlaceholder(placeholder, { exact: true });
      }
    }
    if ((await search.count()) !== 1) return { status: "not_found", query_attempts: 0, exact_name_matches: 0 };
    await search.fill(expectedName);
    for (let queryAttempt = 1; queryAttempt <= 3; queryAttempt += 1) {
      await search.press("Enter");
      for (let attempt = 0; attempt < 20; attempt += 1) {
        let row = this.page.locator("tr.ovui-tr").filter({ hasText: expectedName });
        if ((await row.count()) === 0) {
          row = this.page.getByRole("row").filter({ hasText: expectedName });
        }
        if ((await row.count()) === 1) {
          const match = (await row.innerText()).match(/ID[:：]\s*(\d+)/);
          if (match?.[1]) {
            const fieldReconciliation = plan.plan_kind === "promotion_create"
              ? await this.reconcilePromotionFields(plan, row)
              : plan.plan_kind === "project_create"
                ? this.reconcileProjectSubmission(plan)
                : undefined;
            return {
              status: "matched",
              created_object_id: match[1],
              query_attempts: queryAttempt,
              exact_name_matches: 1,
              ...(fieldReconciliation ? { field_reconciliation: fieldReconciliation } : {}),
            };
          }
        }
        await this.page.waitForTimeout(250);
      }
    }
    return { status: "not_found", query_attempts: 3, exact_name_matches: 0 };
  }

  private reconcileProjectSubmission(plan: OceanEngineFormPlan): FieldReconciliation {
    const fieldKey = "project.optimization_target_reference";
    const expected = plan.parent_context.optimization_target_external_action;
    if (!expected || !this.submittedExternalAction) {
      return { status: "not_checked", fields: [{ field_key: fieldKey, ...(expected ? { expected } : {}), status: "not_checked" }] };
    }
    const status = expected === this.submittedExternalAction ? "matched" : "drifted";
    return { status, fields: [{ field_key: fieldKey, expected, observed: this.submittedExternalAction, status }] };
  }

  private async reconcilePromotionFields(plan: OceanEngineFormPlan, row: Locator): Promise<FieldReconciliation> {
    const landingFieldKey = "promotion.landing_page_reference";
    const landingExpected = stableReferenceID(plan.steps.find((step) => step.field_key === landingFieldKey)?.value);
    const callToActionFieldKey = "promotion.call_to_action";
    const callToActionValue = plan.steps.find((step) => step.field_key === callToActionFieldKey)?.value;
    const callToActionExpected = Array.isArray(callToActionValue) ? callToActionValue.map(String) : undefined;
    const notChecked = (): FieldReconciliation => ({
      status: "not_checked",
      fields: [
        { field_key: landingFieldKey, ...(landingExpected ? { expected: landingExpected } : {}), status: "not_checked" },
        { field_key: callToActionFieldKey, ...(callToActionExpected ? { expected: callToActionExpected } : {}), status: "not_checked" },
      ],
    });

    const edit = row.getByText("编辑", { exact: true });
    if ((await edit.count()) !== 1) {
      return notChecked();
    }

    const pagePromise = this.page.context().waitForEvent("page", { timeout: 5_000 }).catch(() => undefined);
    await edit.click({ noWaitAfter: true });
    const editPage = await pagePromise;
    if (!editPage) {
      return notChecked();
    }

    try {
      await editPage.waitForLoadState("domcontentloaded", { timeout: 10_000 }).catch(() => undefined);
      const fields: FieldReconciliation["fields"] = [];
      let landingInput = editPage.getByPlaceholder(/落地页链接/);
      for (let attempt = 0; attempt < 40 && (await landingInput.count()) === 0; attempt += 1) {
        await editPage.waitForTimeout(250);
        landingInput = editPage.getByPlaceholder(/落地页链接/);
      }
      const landingObserved = (await landingInput.count()) > 0
        ? landingExpected?.startsWith("http")
          ? (await landingInput.first().inputValue()).trim()
          : await landingInput.first().evaluate((element) => {
            let current: Element | null = element;
            for (let depth = 0; current && depth < 10; depth += 1, current = current.parentElement) {
              const match = current.textContent?.match(/ID[:：]\s*(\d+)/);
              if (match?.[1]) return match[1];
            }
            return undefined;
          })
        : undefined;
      const landingStatus = !landingExpected || !landingObserved
        ? "not_checked"
        : landingObserved === landingExpected ? "matched" : "drifted";
      fields.push({
        field_key: landingFieldKey,
        ...(landingExpected ? { expected: landingExpected } : {}),
        ...(landingObserved ? { observed: landingObserved } : {}),
        status: landingStatus,
      });

      const callToActionObserved = await this.selectedCallToActions(editPage);
      const callToActionStatus = !callToActionExpected || callToActionObserved.length === 0
        ? "not_checked"
        : callToActionObserved.length === callToActionExpected.length && callToActionExpected.every((value) => callToActionObserved.includes(value))
          ? "matched"
          : "drifted";
      fields.push({
        field_key: callToActionFieldKey,
        ...(callToActionExpected ? { expected: callToActionExpected } : {}),
        ...(callToActionObserved.length > 0 ? { observed: callToActionObserved } : {}),
        status: callToActionStatus,
      });

      const status = fields.some((field) => field.status === "drifted")
        ? "drifted"
        : fields.some((field) => field.status === "not_checked") ? "not_checked" : "matched";
      return { status, fields };
    } finally {
      await editPage.close().catch(() => undefined);
    }
  }
}

async function readStdin() {
  return new Promise<string>((resolve, reject) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => { data += chunk; });
    process.stdin.on("end", () => resolve(data));
    process.stdin.on("error", reject);
  });
}

async function main() {
  const args = process.argv.slice(2);
  const option = (name: string) => {
    const index = args.indexOf(name);
    return index >= 0 ? args[index + 1] : undefined;
  };
  // Some npm versions remove an unknown named option but keep its value.
  const sessionFile = option("--session-file") ?? (args[0]?.toLowerCase().endsWith(".json") ? args[0] : undefined);
  const cdpURL = sessionFile ? await resolveSessionPlaywrightEndpoint(sessionFile) : args[0];
  if (!cdpURL || cdpURL.startsWith("--")) {
    throw new Error("Usage: echo PLAN.json | tsx scripts/browser-rpa-runner-v3.ts CDP_URL [--session-file PATH] [--confirm-token TOKEN] [--authority-state-dir DIR]");
  }
  const plan = JSON.parse(await readStdin()) as OceanEngineFormPlan;
  const confirmToken = option("--confirm-token");
  const authorityStateDirectory = option("--authority-state-dir") ?? join(
    process.env.LOCALAPPDATA ?? tmpdir(),
    "cookies",
    "browser-rpa",
    "authority-consumed",
  );
  // Edge can require one explicit external-debugging confirmation.
  // Keep one connection for the complete Runner plan and allow time for it.
  const browser = await chromium.connectOverCDP(cdpURL, { timeout: 120_000 });
  const context = browser.contexts()[0];
  if (!context) throw new Error("the Edge session has no browser context");
  if (args.includes("--reconcile-only")) {
    const page = await context.newPage();
    try {
      const managementURL = new URL("https://ad.oceanengine.com/promotion/promote-manage/project");
      managementURL.searchParams.set("aadvid", plan.account_reference);
      await page.goto(managementURL.toString(), { waitUntil: "domcontentloaded" });
      page.setDefaultTimeout(15_000);
      page.setDefaultNavigationTimeout(15_000);
      const reconciliation = await new PlaywrightPageOperations(page).reconcileSubmit(plan, {
        outcome: "result_unknown",
      });
      const matched = reconciliation.status === "matched";
      writeResultAndExit({
        schema_version: "oceanengine-playwright-rpa-result/v2",
        outcome: matched ? "success" : "failed",
        error_code: matched ? "ok" : "target_effect_not_observed",
        ...(!matched ? { error_message: "the read-only exact-name query did not find the target object" } : {}),
        final_click_performed: false,
        ...(reconciliation.created_object_id ? { created_object_id: reconciliation.created_object_id } : {}),
        reconciliation: reconciliation.status,
        ...(reconciliation.field_reconciliation ? { field_reconciliation: reconciliation.field_reconciliation } : {}),
        steps: [{
          id: "read-only-result-reconciliation",
          status: matched ? "succeeded" : "failed",
          readback: {
            ...reconciliation,
            read_only_reconciliation: true,
            platform_write_performed: false,
          },
        }],
      }, 0);
      return;
    } finally {
      await page.close().catch(() => undefined);
    }
  }
  const page = await resolvePlanPage(context, plan);
  // Keep one drifting selector from consuming the complete Prepare timeout.
  page.setDefaultTimeout(15_000);
  page.setDefaultNavigationTimeout(15_000);
  const result = await executePlan(plan, new PlaywrightPageOperations(page), {
    ...(confirmToken ? { confirmToken } : {}),
    authorityStateDirectory,
    onStepStart: (step) => {
      process.stderr.write(`${JSON.stringify({ event: "runner_step_start", step_id: step.id, kind: step.kind, field_key: step.field_key })}\n`);
    },
  });
  const finalStep = result.steps.at(-1);
  if (finalStep) {
    const current = new URL(page.url());
    finalStep.page_reference = `${current.protocol}//${current.host}${current.pathname}`;
  }
  writeResultAndExit(result, 0);
}

function writeResultAndExit(result: RunnerV3Result, exitCode: number): void {
  const resultFile = commandOption("--result-file");
  if (resultFile) {
    writeFileSync(resultFile, JSON.stringify(result));
    process.exit(exitCode);
  }
  const fallback = setTimeout(() => process.exit(exitCode), 5_000);
  process.stdout.write(JSON.stringify(result), () => {
    clearTimeout(fallback);
    process.exit(exitCode);
  });
}

function commandOption(name: string) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : undefined;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch((error) => {
    const result: RunnerV3Result = {
      schema_version: "oceanengine-playwright-rpa-result/v2",
      outcome: "failed",
      error_code: "runner_failed",
      error_message: error instanceof Error ? error.message : String(error),
      final_click_performed: false,
      steps: [],
    };
    writeResultAndExit(result, 1);
  });
}
