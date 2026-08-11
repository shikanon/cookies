import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = resolve(import.meta.dirname, "..");
const schemaPath = join(root, "docs", "delivery", "schemas", "delivery-platform-configuration-v1.json");
const fixturePath = join(root, "docs", "delivery", "fixtures", "delivery-platform-configuration-v1-valid.json");
const invalidDirectory = join(root, "docs", "delivery", "fixtures");
const canonicalHashTool = join(root, "test", "delivery-platform-configuration-hash.go");
const jcsVectorsPath = join(root, "docs", "delivery", "fixtures", "delivery-platform-configuration-v1-jcs-vectors.json");
const intentSchemaPath = join(root, "docs", "delivery", "schemas", "delivery-intent-v1.json");
const platformV2SchemaPath = join(root, "docs", "delivery", "schemas", "delivery-platform-configuration-v2.json");
const intentFixturePath = join(root, "docs", "delivery", "fixtures", "delivery-intent-v1-valid.json");
const oceanEngineFixturePath = join(root, "docs", "delivery", "fixtures", "delivery-platform-configuration-v2-oceanengine-valid.json");
const magneticEngineFixturePath = join(root, "docs", "delivery", "fixtures", "delivery-platform-configuration-v2-magnetic-pending.json");
const platformV2VectorsPath = join(root, "docs", "delivery", "fixtures", "delivery-platform-contracts-v2-hash-vectors.json");

function readJSON(path: string): Record<string, unknown> {
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>;
}

function setPath(target: Record<string, unknown>, path: string, value: unknown): void {
  const segments = path.split(".");
  let current: unknown = target;
  for (const segment of segments.slice(0, -1)) {
    current = (current as Record<string, unknown>)[segment];
  }
  const leaf = segments.at(-1)!;
  if (Array.isArray(current)) {
    current[Number(leaf)] = value;
  } else {
    (current as Record<string, unknown>)[leaf] = value;
  }
}

function removePath(target: Record<string, unknown>, path: string): void {
  const segments = path.split(".");
  let current: unknown = target;
  for (const segment of segments.slice(0, -1)) {
    current = (current as Record<string, unknown>)[segment];
  }
  const leaf = segments.at(-1)!;
  if (Array.isArray(current)) {
    current.splice(Number(leaf), 1);
  } else {
    delete (current as Record<string, unknown>)[leaf];
  }
}

function fixtureWithMutations(fileName: string): Record<string, unknown> {
  const descriptor = readJSON(join(invalidDirectory, fileName));
  const baseFixture = typeof descriptor.fixture === "string" ? join(invalidDirectory, descriptor.fixture) : fixturePath;
  const candidate = structuredClone(readJSON(baseFixture));
  for (const mutation of descriptor.mutations as Array<{ path: string; value?: unknown; remove?: boolean }>) {
    if (mutation.remove) {
      removePath(candidate, mutation.path);
    } else {
      setPath(candidate, mutation.path, mutation.value);
    }
  }
  return candidate;
}

function canonicalHash(payload: unknown): string {
  const output = execFileSync("go", ["run", canonicalHashTool], { input: JSON.stringify(payload), encoding: "utf8" });
  return output.trim();
}

function contractCanonicalHash(kind: "intent" | "platform", envelope: unknown): string {
  const output = execFileSync("go", ["run", canonicalHashTool, kind], { input: JSON.stringify(envelope), encoding: "utf8" });
  return output.trim();
}

const ajv = new Ajv2020({ allErrors: true, strict: true });
addFormats(ajv);
const validate = ajv.compile(readJSON(schemaPath));
const intentSchema = readJSON(intentSchemaPath);
ajv.addSchema(intentSchema);
const validateIntent = ajv.getSchema(intentSchema.$id as string)!;
const validatePlatformV2 = ajv.compile(readJSON(platformV2SchemaPath));

test("delivery configuration schema resolves internal refs and validates the v1 fixture", () => {
  const fixture = readJSON(fixturePath);
  assert.equal(validate(fixture), true, JSON.stringify(validate.errors));
  const payload = fixture.payload as Record<string, unknown>;
  assert.equal(fixture.canonical_hash, canonicalHash(payload));
  assert.equal("canonical_hash" in payload, false);
  assert.equal("compilation_metadata" in payload, false);
});

test("compilation metadata is outside the hash projection", () => {
  const fixture = readJSON(fixturePath);
  const changed = structuredClone(fixture);
  (changed.compilation_metadata as Record<string, unknown>).evidence_states = ["platform_pending"];
  (changed.compilation_metadata as Record<string, unknown>).evidence_refs = ["fixture://changed-evidence"];
  assert.deepEqual(changed.payload, fixture.payload);
  assert.equal(canonicalHash(changed.payload), canonicalHash(fixture.payload));
});

test("invalid fixtures fail schema validation for the intended reason", () => {
  for (const fileName of [
    "delivery-platform-configuration-v1-invalid-hash.json",
    "delivery-platform-configuration-v1-invalid-metadata.json",
    "delivery-platform-configuration-v1-invalid-action-boundary.json",
    "delivery-platform-configuration-v1-invalid-redundant-parent.json",
    "delivery-platform-configuration-v1-invalid-resolved-reference.json",
    "delivery-platform-configuration-v1-invalid-condition-result.json"
  ]) {
    const descriptor = readJSON(join(invalidDirectory, fileName));
    const expectedError = descriptor.expected_error as string;
    const candidate = fixtureWithMutations(fileName);
    assert.equal(validate(candidate), false, fileName);
    assert.ok(
      validate.errors?.some((error) => error.message?.includes(expectedError)),
      `${fileName} expected error "${expectedError}" but got ${JSON.stringify(validate.errors)}`
    );
  }
});

test("blocked condition results are representable (result:false + blocked_reason)", () => {
  const candidate = structuredClone(readJSON(fixturePath));
  (candidate.compilation_metadata as Record<string, unknown>).conditions = [
    {
      condition: { field: "payload.platform_project.carrier", equals: "orange_landing_page" },
      result: false,
      evidence_states: ["observed"],
      blocked_reason: "owner must confirm carrier matches page context"
    }
  ];
  assert.equal(validate(candidate), true, JSON.stringify(validate.errors));
});

test("canonical hash is sensitive to payload field changes", () => {
  const fixture = readJSON(fixturePath);
  const changed = structuredClone(fixture);
  setPath(changed.payload as Record<string, unknown>, "platform_project.project_name", "mutated-project-name");
  assert.notEqual(canonicalHash(changed.payload), canonicalHash(fixture.payload as Record<string, unknown>));
});

test("canonical hash fixtures match production RFC8785 output", () => {
  const vectorFixture = readJSON(jcsVectorsPath);
  const vectors = vectorFixture.vectors as Array<{ name: string; payload: unknown; expected_canonical_hash: string }>;
  for (const vector of vectors) {
    assert.equal(canonicalHash(vector.payload), vector.expected_canonical_hash, vector.name);
  }
});

test("platform-neutral intent validates and hashes only its canonical business payload", () => {
  const fixture = readJSON(intentFixturePath);
  assert.equal(validateIntent(fixture), true, JSON.stringify(validateIntent.errors));
  assert.equal(fixture.canonical_hash, contractCanonicalHash("intent", fixture));

  const metadataOnlyChange = structuredClone(fixture);
  (metadataOnlyChange.audit as Record<string, unknown>).created_by = "another-user";
  (metadataOnlyChange.configuration_provenance as Record<string, unknown>).kind = "decision_engine";
  (metadataOnlyChange.fact_provenance as Record<string, unknown>).source = "connector";
  setPath(metadataOnlyChange.payload as Record<string, unknown>, "product_references.0.display_name_snapshot", "renamed product");
  assert.equal(contractCanonicalHash("intent", metadataOnlyChange), fixture.canonical_hash);

  const businessChange = structuredClone(fixture);
  setPath(businessChange.payload as Record<string, unknown>, "marketing_objective", "increase revenue");
  assert.notEqual(contractCanonicalHash("intent", businessChange), fixture.canonical_hash);

  const encodedPayload = JSON.stringify(fixture.payload);
  for (const platformTerm of ["ocean_engine", "magnetic_engine", "carrier", "delivery_mode", "promotion"]) {
    assert.equal(encodedPayload.includes(platformTerm), false, platformTerm);
  }
});

test("tagged v2 platform fixtures validate without a compatibility projector", () => {
  const oceanEngine = readJSON(oceanEngineFixturePath);
  const magneticEngine = readJSON(magneticEngineFixturePath);
  assert.equal(validatePlatformV2(oceanEngine), true, JSON.stringify(validatePlatformV2.errors));
  assert.equal(validatePlatformV2(magneticEngine), true, JSON.stringify(validatePlatformV2.errors));
  assert.equal(oceanEngine.canonical_hash, contractCanonicalHash("platform", oceanEngine));
  assert.equal(magneticEngine.canonical_hash, contractCanonicalHash("platform", magneticEngine));

  const oceanProfile = (oceanEngine.payload as Record<string, unknown>).ocean_engine as Record<string, unknown>;
  assert.equal(Array.isArray(oceanProfile.promotions), true);
  assert.equal((oceanProfile.promotions as unknown[]).length, 2);
  for (const promotion of oceanProfile.promotions as Array<Record<string, unknown>>) {
    assert.equal("parent_project_id" in promotion, false);
    assert.equal("project_id" in promotion, false);
  }
  const magneticProfile = JSON.stringify((magneticEngine.payload as Record<string, unknown>).magnetic_engine);
  for (const invented of ["project", "promotion", "selector"]) {
    assert.equal(magneticProfile.includes(invented), false, invented);
  }
});

test("v2 platform hash binds intent identity and business fields but excludes audit evidence", () => {
  const fixture = readJSON(oceanEngineFixturePath);
  const metadataOnlyChange = structuredClone(fixture);
  (metadataOnlyChange.audit as Record<string, unknown>).created_by = "another-user";
  (metadataOnlyChange.configuration_provenance as Record<string, unknown>).kind = "import";
  (metadataOnlyChange.fact_provenance as Record<string, unknown>).source = "connector";
  (metadataOnlyChange.compilation_metadata as Record<string, unknown>).evidence_refs = ["page://changed"];
  setPath(metadataOnlyChange.payload as Record<string, unknown>, "ocean_engine.project.account_reference.display_name_snapshot", "renamed account");
  assert.equal(contractCanonicalHash("platform", metadataOnlyChange), fixture.canonical_hash);

  const intentVersionChange = structuredClone(fixture);
  setPath(intentVersionChange, "intent.version_number", 2);
  assert.notEqual(contractCanonicalHash("platform", intentVersionChange), fixture.canonical_hash);

  const projectChange = structuredClone(fixture);
  setPath(projectChange.payload as Record<string, unknown>, "ocean_engine.project.project_name", "changed project");
  assert.notEqual(contractCanonicalHash("platform", projectChange), fixture.canonical_hash);
});

test("new contract invalid descriptors fail for their stated reason", () => {
  const cases = [
    { fileName: "delivery-intent-v1-invalid-resolved-reference.json", validator: validateIntent },
    { fileName: "delivery-platform-configuration-v2-invalid-discriminator.json", validator: validatePlatformV2 },
    { fileName: "delivery-platform-configuration-v2-invalid-unknown-schema.json", validator: validatePlatformV2 },
    { fileName: "delivery-platform-configuration-v2-invalid-unknown-profile.json", validator: validatePlatformV2 },
    { fileName: "delivery-platform-configuration-v2-invalid-missing-project.json", validator: validatePlatformV2 },
    { fileName: "delivery-platform-configuration-v2-invalid-promotion-reference.json", validator: validatePlatformV2 }
  ];
  for (const { fileName, validator } of cases) {
    const descriptor = readJSON(join(invalidDirectory, fileName));
    const expectedError = descriptor.expected_error as string;
    const candidate = fixtureWithMutations(fileName);
    assert.equal(validator(candidate), false, fileName);
    assert.ok(
      validator.errors?.some((error) => error.message?.includes(expectedError)),
      `${fileName} expected error "${expectedError}" but got ${JSON.stringify(validator.errors)}`
    );
  }
});

test("v2 contract hash vectors match production projections", () => {
  const vectors = readJSON(platformV2VectorsPath).vectors as Array<{
    name: string;
    kind: "intent" | "platform";
    fixture: string;
    expected_canonical_hash: string;
  }>;
  for (const vector of vectors) {
    const fixture = readJSON(join(invalidDirectory, vector.fixture));
    assert.equal(contractCanonicalHash(vector.kind, fixture), vector.expected_canonical_hash, vector.name);
  }
});
