import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";

import { type EcommerceParentConditionManifest } from "../scripts/oceanengine-ecommerce-field-compiler.ts";
import {
  compileOceanEngineFormPlan,
  type OceanEnginePlanCompilerInput,
} from "../scripts/oceanengine-form-plan-compiler.ts";
import {
  AuthorityError,
  authorizeSubmitPlan,
  consumeSubmitAuthority,
  validateSubmitAuthority,
} from "../scripts/rpa-authority.ts";

const root = resolve(import.meta.dirname, "..");
const readJSON = (relative: string) => JSON.parse(readFileSync(resolve(root, relative), "utf8"));
const manifest = readJSON("docs/delivery/fixtures/oceanengine-ecommerce-parent-condition-manifest-v1.json") as EcommerceParentConditionManifest;
const planSchema = readJSON("docs/delivery/schemas/oceanengine-playwright-rpa-plan-v3.json");
const authoritySchema = readJSON("docs/delivery/schemas/oceanengine-execution-authority-v1.json");
const ajv = new Ajv2020({ allErrors: true, strict: false });
ajv.addSchema(authoritySchema);
const validatePlan = ajv.compile(planSchema);

function prepareProjectPlan() {
  const input = readJSON("docs/delivery/fixtures/oceanengine-project-create-plan-input-v1.json") as OceanEnginePlanCompilerInput;
  input.account_reference = "1855554434276391";
  input.values["project.schedule"] = { start: "2026-08-26", end: "2026-08-26" };
  return compileOceanEngineFormPlan("project_create", manifest, input);
}

test("authority converts a ready prepare plan into a schema-valid submit plan", () => {
  const now = new Date("2026-08-25T01:00:00.000Z");
  const preparePlan = prepareProjectPlan();
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  assert.equal(validatePlan(bundle.plan), true, JSON.stringify(validatePlan.errors));
  assert.equal(bundle.plan.mode, "submit");
  assert.equal(bundle.plan.allow_remote_write, true);
  assert.equal(bundle.plan.maximum_final_clicks, 1);
  assert.equal(bundle.plan.steps.at(-1)?.blocked, false);
  assert.ok(bundle.confirm_token.length >= 24);
  assert.doesNotThrow(() => validateSubmitAuthority(bundle.plan, bundle.confirm_token, now));
});

test("authority accepts a date range when its start date matches", () => {
  const now = new Date("2026-08-25T01:00:00.000Z");
  const preparePlan = prepareProjectPlan();
  const schedule = preparePlan.steps.find((step) => step.field_key === "project.schedule");
  assert.ok(schedule);
  schedule.value = { start: "2026-08-26", end: "2026-08-27" };
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  assert.doesNotThrow(() => validateSubmitAuthority(bundle.plan, bundle.confirm_token, now));
});

test("authority rejects plan changes, expiration, and money above the limit", () => {
  const now = new Date("2026-08-25T01:00:00.000Z");
  const preparePlan = prepareProjectPlan();
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
    ttl_seconds: 60,
  }, now);

  const changed = structuredClone(bundle.plan);
  const budget = changed.steps.find((step) => step.field_key === "project.daily_budget");
  assert.ok(budget);
  budget.value = 301;
  assert.throws(
    () => validateSubmitAuthority(changed, bundle.confirm_token, now),
    (error) => error instanceof AuthorityError && error.code === "authority_plan_mismatch",
  );
  assert.throws(
    () => validateSubmitAuthority(bundle.plan, bundle.confirm_token, new Date("2026-08-25T01:02:00.000Z")),
    (error) => error instanceof AuthorityError && error.code === "authority_expired",
  );
  assert.throws(
    () => authorizeSubmitPlan(prepareProjectPlan(), {
      account_reference: preparePlan.account_reference,
      maximum_money_cny: 299,
      schedule_date: "2026-08-26",
    }, now),
    (error) => error instanceof AuthorityError && error.code === "authority_money_limit",
  );
});

test("authority consumption is atomic and one-time", async () => {
  const now = new Date("2026-08-25T01:00:00.000Z");
  const preparePlan = prepareProjectPlan();
  const bundle = authorizeSubmitPlan(preparePlan, {
    account_reference: preparePlan.account_reference,
    maximum_money_cny: 300,
    schedule_date: "2026-08-26",
  }, now);
  const authority = validateSubmitAuthority(bundle.plan, bundle.confirm_token, now);
  const stateDirectory = await mkdtemp(join(tmpdir(), "cookies-authority-test-"));
  try {
    await consumeSubmitAuthority(authority, stateDirectory);
    await assert.rejects(
      consumeSubmitAuthority(authority, stateDirectory),
      (error) => error instanceof AuthorityError && error.code === "authority_already_used",
    );
  } finally {
    await rm(stateDirectory, { recursive: true, force: true });
  }
});
