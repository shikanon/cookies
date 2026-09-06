import { createHash, randomBytes, randomUUID } from "node:crypto";
import { mkdir, open, readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

import type {
  OceanEngineExecutionAuthority,
  OceanEngineFormPlan,
  OceanEnginePlanKind,
} from "./oceanengine-form-plan-compiler.ts";

export type SubmitAuthorityConstraints = {
  account_reference: string;
  maximum_money_cny: number;
  schedule_date: string;
  ttl_seconds?: number;
};

export type AuthorizedSubmitBundle = {
  plan: OceanEngineFormPlan;
  confirm_token: string;
};

export class AuthorityError extends Error {
  constructor(readonly code: string, message: string) {
    super(message);
  }
}

function canonicalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalize);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, child]) => [key, canonicalize(child)]),
    );
  }
  return value;
}

export function canonicalJSON(value: unknown) {
  return JSON.stringify(canonicalize(value));
}

export function sha256(value: string) {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

function submitBinding(plan: OceanEngineFormPlan) {
  const { execution_authority: _authority, ...binding } = plan;
  return binding;
}

export function submitPlanSha256(plan: OceanEngineFormPlan) {
  return sha256(canonicalJSON(submitBinding(plan)));
}

function validateDate(value: string, label: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    throw new AuthorityError("invalid_authority", `${label} must use YYYY-MM-DD`);
  }
}

function moneyValues(plan: OceanEngineFormPlan) {
  return plan.steps.flatMap((step) => {
    if (step.kind !== "field_action" || step.value_state !== "provided" || !step.field_key) return [];
    if (!/(?:budget|bid|coefficient)$/.test(step.field_key)) return [];
    return [{ key: step.field_key, value: Number(step.value) }];
  });
}

function scheduleValues(plan: OceanEngineFormPlan) {
  return plan.steps.flatMap((step) => {
    if (step.field_key !== "project.schedule" || !step.value || typeof step.value !== "object") return [];
    const value = step.value as { start?: unknown };
    return [String(value.start ?? "")];
  });
}

function assertConstraints(plan: OceanEngineFormPlan, authority: OceanEngineExecutionAuthority) {
  if (!/^\d+$/.test(plan.account_reference)) {
    throw new AuthorityError("unstable_account_reference", "submit plans need an exact numeric account reference");
  }
  const needsObject = plan.plan_kind.endsWith("_edit");
  const needsParent = plan.plan_kind.startsWith("promotion_");
  if (needsObject && !/^\d+$/.test(plan.object_reference ?? "")) {
    throw new AuthorityError("unstable_object_reference", "edit submit plans need an exact numeric object reference");
  }
  if (needsParent && !/^\d+$/.test(plan.parent_project_reference ?? "")) {
    throw new AuthorityError("unstable_parent_reference", "promotion submit plans need an exact numeric parent project reference");
  }
  if (plan.account_reference !== authority.account_reference) {
    throw new AuthorityError("account_mismatch", "authority account does not match the plan");
  }
  if (plan.plan_kind !== authority.permitted_plan_kind) {
    throw new AuthorityError("authority_scope_mismatch", "authority plan kind does not match the plan");
  }
  for (const money of moneyValues(plan)) {
    if (!Number.isFinite(money.value) || money.value < 0 || money.value > authority.maximum_money_cny) {
      throw new AuthorityError("authority_money_limit", `${money.key} exceeds the authority limit`);
    }
  }
  const schedules = scheduleValues(plan);
  if (schedules.some((date) => date !== authority.schedule_date)) {
    throw new AuthorityError("authority_schedule_mismatch", "plan start date does not match the authority date");
  }
}

function toSubmitBase(preparePlan: OceanEngineFormPlan): OceanEngineFormPlan {
  if (
    preparePlan.schema_version !== "oceanengine-playwright-rpa-plan/v3" ||
    preparePlan.mode !== "prepare" ||
    preparePlan.status !== "ready" ||
    preparePlan.allow_remote_write ||
    preparePlan.maximum_final_clicks !== 0
  ) {
    throw new AuthorityError("invalid_prepare_plan", "authority needs a ready prepare plan");
  }
  const boundary = preparePlan.steps.at(-1);
  if (boundary?.kind !== "final_click_boundary" || !boundary.blocked || !boundary.remote_write) {
    throw new AuthorityError("invalid_prepare_plan", "prepare plan has no blocked final-click boundary");
  }
  return {
    ...preparePlan,
    mode: "submit",
    allow_remote_write: true,
    maximum_final_clicks: 1,
    steps: preparePlan.steps.map((step) => step.kind === "final_click_boundary"
      ? { ...step, blocked: false, block_reason: undefined }
      : step),
  };
}

export function authorizeSubmitPlan(
  preparePlan: OceanEngineFormPlan,
  constraints: SubmitAuthorityConstraints,
  now = new Date(),
): AuthorizedSubmitBundle {
  if (!constraints.account_reference.trim()) {
    throw new AuthorityError("invalid_authority", "account_reference is required");
  }
  if (!Number.isFinite(constraints.maximum_money_cny) || constraints.maximum_money_cny <= 0) {
    throw new AuthorityError("invalid_authority", "maximum_money_cny must be positive");
  }
  validateDate(constraints.schedule_date, "schedule_date");
  const ttlSeconds = constraints.ttl_seconds ?? 600;
  if (!Number.isInteger(ttlSeconds) || ttlSeconds < 30 || ttlSeconds > 900) {
    throw new AuthorityError("invalid_authority", "ttl_seconds must be between 30 and 900");
  }

  const submitPlan = toSubmitBase(preparePlan);
  const token = randomBytes(24).toString("base64url");
  const authority: OceanEngineExecutionAuthority = {
    schema_version: "browser-rpa-execution-authority/v1",
    authority_id: randomUUID(),
    plan_sha256: submitPlanSha256(submitPlan),
    confirm_token_sha256: sha256(token),
    issued_at: now.toISOString(),
    expires_at: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
    account_reference: constraints.account_reference,
    permitted_plan_kind: submitPlan.plan_kind,
    maximum_money_cny: constraints.maximum_money_cny,
    schedule_date: constraints.schedule_date,
    maximum_final_clicks: 1,
  };
  assertConstraints(submitPlan, authority);
  return { plan: { ...submitPlan, execution_authority: authority }, confirm_token: token };
}

export function validateSubmitAuthority(
  plan: OceanEngineFormPlan,
  confirmToken: string,
  now = new Date(),
) {
  const authority = plan.execution_authority;
  if (!authority || authority.schema_version !== "browser-rpa-execution-authority/v1") {
    throw new AuthorityError("authority_missing", "submit plan has no execution authority");
  }
  if (plan.mode !== "submit" || !plan.allow_remote_write || plan.maximum_final_clicks !== 1) {
    throw new AuthorityError("write_blocked", "submit plan does not allow one final click");
  }
  if (plan.status !== "ready" || plan.blocked_reasons.length > 0) {
    throw new AuthorityError("plan_blocked", "blocked plans cannot be submitted");
  }
  if (plan.steps.at(-1)?.kind !== "final_click_boundary" || plan.steps.at(-1)?.blocked) {
    throw new AuthorityError("invalid_plan", "submit plan has no enabled final-click boundary");
  }
  if (submitPlanSha256(plan) !== authority.plan_sha256) {
    throw new AuthorityError("authority_plan_mismatch", "plan changed after authority was issued");
  }
  if (sha256(confirmToken) !== authority.confirm_token_sha256) {
    throw new AuthorityError("authority_token_mismatch", "confirmation token does not match");
  }
  const issuedAt = Date.parse(authority.issued_at);
  const expiresAt = Date.parse(authority.expires_at);
  if (!Number.isFinite(issuedAt) || !Number.isFinite(expiresAt) || now.getTime() < issuedAt || now.getTime() > expiresAt) {
    throw new AuthorityError("authority_expired", "execution authority is not active");
  }
  if (expiresAt - issuedAt > 900_000) {
    throw new AuthorityError("invalid_authority", "execution authority lifetime exceeds 15 minutes");
  }
  assertConstraints(plan, authority);
  return authority;
}

export async function consumeSubmitAuthority(
  authority: OceanEngineExecutionAuthority,
  stateDirectory: string,
) {
  await mkdir(stateDirectory, { recursive: true });
  const path = resolve(stateDirectory, `${authority.authority_id}.used.json`);
  let handle;
  try {
    handle = await open(path, "wx");
    await handle.writeFile(JSON.stringify({
      schema_version: "browser-rpa-authority-consumption/v1",
      authority_id: authority.authority_id,
      plan_sha256: authority.plan_sha256,
      consumed_at: new Date().toISOString(),
    }, null, 2));
  } catch (error) {
    const code = error && typeof error === "object" && "code" in error ? String(error.code) : "";
    if (code === "EEXIST") throw new AuthorityError("authority_already_used", "execution authority was already used");
    throw error;
  } finally {
    await handle?.close();
  }
  return path;
}

function option(args: string[], name: string) {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
}

async function readStdin() {
  return new Promise<string>((resolveInput, rejectInput) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => { data += chunk; });
    process.stdin.on("end", () => resolveInput(data));
    process.stdin.on("error", rejectInput);
  });
}

async function main() {
  const [command, planPath, ...args] = process.argv.slice(2);
  if (command !== "issue" || !planPath) {
    throw new Error("Usage: tsx scripts/rpa-authority.ts issue PLAN.json --account-reference ID --maximum-money CNY --schedule-date YYYY-MM-DD");
  }
  const accountReference = option(args, "--account-reference");
  const maximumMoney = option(args, "--maximum-money");
  const scheduleDate = option(args, "--schedule-date");
  const ttl = option(args, "--ttl-seconds");
  if (!accountReference || !maximumMoney || !scheduleDate) throw new Error("authority options are incomplete");
  const planSource = planPath === "-" ? await readStdin() : await readFile(planPath, "utf8");
  const plan = JSON.parse(planSource) as OceanEngineFormPlan;
  const bundle = authorizeSubmitPlan(plan, {
    account_reference: accountReference,
    maximum_money_cny: Number(maximumMoney),
    schedule_date: scheduleDate,
    ...(ttl ? { ttl_seconds: Number(ttl) } : {}),
  });
  process.stdout.write(`${JSON.stringify(bundle, null, 2)}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch((error) => {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  });
}

export type { OceanEnginePlanKind };
