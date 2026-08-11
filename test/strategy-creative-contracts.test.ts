import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import test from "node:test";
import Ajv2020, { type ValidateFunction } from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {
  CREATIVE_CONTRACT_VERSIONS,
  CREATIVE_ROUTE_PROFILES,
  CREATIVE_SHARED_WORKFLOW_VERSION,
  CREATIVE_STATE_MACHINE,
  canTransitionCreativeState,
} from "../src/contracts/creative.js";

const repositoryRoot = resolve(import.meta.dirname, "..");
const contractsDirectory = join(repositoryRoot, "api", "contracts");
const fixturesDirectory = join(repositoryRoot, "api", "fixtures");
const openAPIDirectory = join(repositoryRoot, "api", "openapi");

const fixtureContracts = [
  ["strategy-creative-handoff-v1-ready.json", "strategy-creative-handoff-v1.schema.json"],
  ["strategy-creative-handoff-v1-blocked.json", "strategy-creative-handoff-v1.schema.json"],
  ["creative-intake-create-v2.json", "creative-intake-create-v2.schema.json"],
  ["creative-intake-v2-ready.json", "creative-intake-v2.schema.json"],
  ["strategy-creative-task-plan-v2-ready.json", "strategy-creative-task-plan-v2.schema.json"],
  ["strategy-creative-task-strategy-v2-ready.json", "strategy-creative-task-strategy-v2.schema.json"],
  ["strategy-creative-task-overlay-v1-ready.json", "strategy-creative-task-overlay-v1.schema.json"],
  ["creative-intake-create-v3-base.json", "creative-intake-create-v3.schema.json"],
  ["creative-intake-create-v3-overlay.json", "creative-intake-create-v3.schema.json"],
  ["creative-intake-create-v3-overlay-mismatch.json", "creative-intake-create-v3.schema.json"],
  ["creative-intake-v3-ready.json", "creative-intake-v3.schema.json"],
  ["creative-planning-context-v1-enhanced.json", "creative-planning-context-v1.schema.json"],
  ["creative-direction-v1-candidate.json", "creative-direction-v1.schema.json"],
  ["creative-direction-candidate-batch-v1-ready.json", "creative-direction-candidate-batch-v1.schema.json"],
  ["creative-direction-candidate-batch-v1-failed.json", "creative-direction-candidate-batch-v1.schema.json"],
  ["creative-shared-workflow-v1-frozen.json", "creative-shared-workflow-v1.schema.json"],
  ["creative-image-text-draft-v2.json", "creative-image-text-draft-v2.schema.json"],
  ["creative-image-prompt-package-v1.json", "creative-image-prompt-package-v1.schema.json"],
  ["creative-image-generation-attempt-v1.json", "creative-image-generation-attempt-v1.schema.json"],
  ["creative-image-slot-selection-v1.json", "creative-image-slot-selection-v1.schema.json"],
  ["creative-image-render-spec-v1.json", "creative-image-render-spec-v1.schema.json"],
  ["creative-image-text-workspace-v1.json", "creative-image-text-workspace-v1.schema.json"],
  ["creative-production-run-page-v1.json", "creative-production-center-v1.schema.json"],
  ["creative-production-run-detail-v1.json", "creative-production-center-v1.schema.json"],
  ["creative-production-asset-page-v1.json", "creative-production-center-v1.schema.json"],
  ["creative-production-retry-v1.json", "creative-production-center-v1.schema.json"],
] as const;

const ajv = new Ajv2020({
  allErrors: true,
  allowUnionTypes: true,
  strict: true,
  strictRequired: false,
});
addFormats(ajv);

for (const filename of readdirSync(contractsDirectory).filter((name) => name.endsWith(".schema.json"))) {
  ajv.addSchema(readJSON(join(contractsDirectory, filename)));
}

function readJSON(path: string): Record<string, unknown> {
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>;
}

function validatorFor(schemaFilename: string): ValidateFunction {
  const schema = readJSON(join(contractsDirectory, schemaFilename));
  const validator = ajv.getSchema(String(schema.$id));
  assert.ok(validator, `schema ${schemaFilename} was not registered`);
  return validator;
}

function assertValidFixture(fixtureFilename: string, schemaFilename: string): void {
  const fixture = readJSON(join(fixturesDirectory, fixtureFilename));
  const validate = validatorFor(schemaFilename);
  assert.equal(
    validate(fixture),
    true,
    `${fixtureFilename} does not satisfy ${schemaFilename}: ${ajv.errorsText(validate.errors, { separator: "\n" })}`,
  );
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

test("frozen Strategy-to-Creative fixtures satisfy their declared JSON Schemas", () => {
  for (const [fixture, schema] of fixtureContracts) {
    assertValidFixture(fixture, schema);
  }
});

test("strict contracts reject additional properties, missing route lineage, and creative leakage", () => {
  const create = readJSON(join(fixturesDirectory, "creative-intake-create-v3-base.json"));
  const validateCreate = validatorFor("creative-intake-create-v3.schema.json");

  const additionalProperty = { ...create, route_index: 0 };
  assert.equal(validateCreate(additionalProperty), false, "v3 create accepted route_index");

  const missingRoute = clone(create);
  delete missingRoute.selected_route_id;
  assert.equal(validateCreate(missingRoute), false, "v3 create accepted missing selected_route_id");

  const overlay = readJSON(join(fixturesDirectory, "strategy-creative-task-overlay-v1-ready.json"));
  const validateOverlay = validatorFor("strategy-creative-task-overlay-v1.schema.json");
  assert.equal(
    validateOverlay({ ...overlay, concept: "must remain Creative-owned" }),
    false,
    "overlay accepted a Creative-owned concept",
  );

  const taskStrategy = readJSON(join(fixturesDirectory, "strategy-creative-task-strategy-v2-ready.json"));
  const validateTaskStrategy = validatorFor("strategy-creative-task-strategy-v2.schema.json");
  const leakingTaskStrategy = clone(taskStrategy);
  leakingTaskStrategy.business_strategy = {
    ...(leakingTaskStrategy.business_strategy as Record<string, unknown>),
    hook: "must remain Creative-owned",
  };
  assert.equal(
    validateTaskStrategy(leakingTaskStrategy),
    false,
    "task strategy accepted a Creative-owned hook",
  );
});

test("the mismatch fixture is structurally valid but carries a deliberate route-lineage conflict", () => {
  const mismatch = readJSON(join(fixturesDirectory, "creative-intake-create-v3-overlay-mismatch.json"));
  const overlay = readJSON(join(fixturesDirectory, "strategy-creative-task-overlay-v1-ready.json"));
  const validate = validatorFor("creative-intake-create-v3.schema.json");

  assert.equal(validate(mismatch), true, ajv.errorsText(validate.errors));
  assert.notEqual(mismatch.selected_route_id, overlay.selected_route_id);
});

test("the frontend consumes the frozen Creative contract versions and state graph", () => {
  const frozen = readJSON(join(fixturesDirectory, "creative-shared-workflow-v1-frozen.json"));

  assert.equal(frozen.contract_version, CREATIVE_SHARED_WORKFLOW_VERSION);
  assert.deepEqual(frozen.contracts, CREATIVE_CONTRACT_VERSIONS);
  assert.deepEqual(frozen.route_profiles, CREATIVE_ROUTE_PROFILES);
  assert.deepEqual(frozen.states, CREATIVE_STATE_MACHINE);

  assert.equal(canTransitionCreativeState("intake", "ready", "superseded"), true);
  assert.equal(canTransitionCreativeState("direction", "confirmed", "candidate"), false);
  assert.equal(canTransitionCreativeState("task", "archived", "draft"), false);
  assert.equal(canTransitionCreativeState("creative_version", "checked", "approved"), true);
});

test("shared workflow stays format-neutral while format contracts own their schemas", () => {
  const frozen = readJSON(join(fixturesDirectory, "creative-shared-workflow-v1-frozen.json"));
  const validateFrozen = validatorFor("creative-shared-workflow-v1.schema.json");

  assert.deepEqual(Object.keys(frozen.route_profiles as object).sort(), ["brand_video", "image_text"]);
  assert.deepEqual(
    Object.keys(frozen.contracts as object).sort(),
    ["direction", "direction_candidate_batch", "intake", "intake_create", "planning_context"],
  );

  const formatLeak = clone(frozen);
  (formatLeak as Record<string, unknown>).image_plan = [];
  assert.equal(validateFrozen(formatLeak), false, "shared workflow accepted an image-text draft field");

  const routeDrift = clone(frozen);
  const routeProfiles = routeDrift.route_profiles as Record<string, Record<string, unknown>>;
  routeProfiles.image_text.performance_mode = "brand_video";
  assert.equal(validateFrozen(routeDrift), false, "image-text route accepted brand-video mode");

  const versionedFormatContracts = [
    ["creative-image-text-draft-v2.schema.json", "creative-image-text-draft/v2"],
    ["creative-image-prompt-package-v1.schema.json", "creative-image-prompt-package/v1"],
    ["creative-image-generation-attempt-v1.schema.json", "creative-image-generation-attempt/v1"],
    ["creative-image-slot-selection-v1.schema.json", "creative-image-slot-selection/v1"],
    ["creative-image-render-spec-v1.schema.json", "creative-image-render-spec/v1"],
    ["creative-image-text-workspace-v1.schema.json", "creative-image-text-workspace/v1"],
  ] as const;
  for (const [schemaFilename, version] of versionedFormatContracts) {
    const schema = readJSON(join(contractsDirectory, schemaFilename));
    const properties = schema.properties as Record<string, Record<string, unknown>>;
    assert.equal(schema.additionalProperties, false, `${schemaFilename} must remain strict`);
    assert.equal(properties.contract_version.const, version, `${schemaFilename} version drifted`);
  }
});

test("shared intake rejects image-text and brand-video implementation details", () => {
  const create = readJSON(join(fixturesDirectory, "creative-intake-create-v3-base.json"));
  const validateCreate = validatorFor("creative-intake-create-v3.schema.json");

  for (const field of ["image_plan", "prompt_package", "script", "storyboard", "provider_parameters"]) {
    assert.equal(
      validateCreate({ ...create, [field]: "must remain format-owned" }),
      false,
      `shared intake accepted ${field}`,
    );
  }
});

test("production center freezes a read-only projection and delegated retry contract", () => {
  const schemaFilename = "creative-production-center-v1.schema.json";
  const schema = readJSON(join(contractsDirectory, schemaFilename));
  const validate = validatorFor(schemaFilename);
  const page = readJSON(join(fixturesDirectory, "creative-production-run-page-v1.json"));
  const detail = readJSON(join(fixturesDirectory, "creative-production-run-detail-v1.json"));
  const assets = readJSON(join(fixturesDirectory, "creative-production-asset-page-v1.json"));
  const retry = readJSON(join(fixturesDirectory, "creative-production-retry-v1.json"));

  for (const [name, value] of Object.entries({ page, detail, assets, retry })) {
    assert.equal(validate(value), true, `${name}: ${ajv.errorsText(validate.errors)}`);
  }

  const pageItems = page.items as Array<Record<string, unknown>>;
  assert.equal(pageItems[0].normalized_status, "partially_succeeded", "partial success was collapsed");

  const leakedReviewState = clone(page);
  ((leakedReviewState.items as Array<Record<string, unknown>>)[0]).quality_check_status = "passed";
  assert.equal(validate(leakedReviewState), false, "production projection accepted review-owned state");

  const leakedStorageLocation = clone(assets);
  const assetItems = leakedStorageLocation.items as Array<Record<string, Record<string, unknown>>>;
  assetItems[0].asset.bucket = "must-not-cross-the-assets-seam";
  assert.equal(validate(leakedStorageLocation), false, "production asset view accepted an object-storage key");

  const unsupportedVisibleKind = clone(page);
  ((unsupportedVisibleKind.items as Array<Record<string, unknown>>)[0]).media_kind = "3d";
  assert.equal(validate(unsupportedVisibleKind), false, "Phase 0 exposed an unapproved 3D production view");

  const definitions = schema.$defs as Record<string, Record<string, unknown>>;
  const problemProperties = definitions.ProductionProblem.properties as Record<string, Record<string, unknown>>;
  assert.deepEqual(problemProperties.code.enum, [
    "PRODUCTION_RUN_NOT_FOUND",
    "PRODUCTION_SOURCE_UNAVAILABLE",
    "PRODUCTION_CURSOR_INVALID",
    "PRODUCTION_RETRY_NOT_ALLOWED",
    "PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW",
    "PRODUCTION_INPUT_ASSET_UNAVAILABLE",
    "PRODUCTION_IDEMPOTENCY_CONFLICT",
  ]);
});

test("Creative OpenAPI exposes only the frozen production query and delegated retry interface", () => {
  const document = readFileSync(join(openAPIDirectory, "creative-v1.yaml"), "utf8");
  const requiredOperations = [
    "operationId: listCreativeProductionRuns",
    "operationId: getCreativeProductionRun",
    "operationId: retryCreativeProductionRun",
    "operationId: listCreativeProductionAssets",
  ];
  for (const operation of requiredOperations) assert.match(document, new RegExp(operation));

  assert.match(document, /creative-production-center-v1\.schema\.json#\/\$defs\/ProductionRunPage/);
  assert.match(document, /PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW/);
  assert.doesNotMatch(document, /operationId: createCreativeProductionRun/);
  assert.doesNotMatch(document, /operationId: approveCreativeProductionRun/);
});

test("every OpenAPI contract reference resolves to a checked-in schema", () => {
  const missing: string[] = [];
  const referencePattern = /\$ref:\s*['"]?([^'"\s}]+)/g;
  const externalValuePattern = /externalValue:\s*['"]?([^'"\s}]+)/g;

  for (const filename of readdirSync(openAPIDirectory).filter((name) => name.endsWith(".yaml"))) {
    const openAPIPath = join(openAPIDirectory, filename);
    const document = readFileSync(openAPIPath, "utf8");
    for (const match of document.matchAll(referencePattern)) {
      const reference = match[1];
      if (!reference.startsWith("../contracts/")) continue;
      const schemaPath = resolve(dirname(openAPIPath), reference.split("#", 1)[0]);
      if (!existsSync(schemaPath)) missing.push(`${filename}: ${reference}`);
    }
    for (const match of document.matchAll(externalValuePattern)) {
      const reference = match[1];
      if (/^[a-z][a-z0-9+.-]*:/i.test(reference)) continue;
      const examplePath = resolve(dirname(openAPIPath), reference);
      if (!existsSync(examplePath)) missing.push(`${filename}: ${reference}`);
    }
  }

  assert.deepEqual(missing, []);
});
