import type {
  BrowserRpaEvidence,
  BrowserRpaEnvironment,
  BrowserRpaLease,
  BrowserRpaProfile,
  BrowserRpaRun,
  BrowserRpaRunStep,
  BrowserRpaRunEvent,
  BrowserRpaSitePolicy,
  ControlledExecutionWorkspace,
  EdgeSessionProbe,
  IssuedFinalConfirmation,
  RunnerV3Plan,
} from './model'

export class ControlledExecutionApiError extends Error {
  constructor(readonly status: number, message: string) {
    super(message)
    this.name = 'ControlledExecutionApiError'
  }
}

const apiPrefix = '/api/platform/v1/browser-rpa/projects'

/**
 * Thin platform-control-plane client. It does not import legacy
 * Delivery execute_mock models. Submit always requires a server-issued
 * one-time confirmation and a fenced session lease.
 */
export const controlledExecutionApi = {
  async listRuns(projectId: string, signal?: AbortSignal): Promise<BrowserRpaRun[]> {
    const response = await request<{ items?: BrowserRpaRun[] }>(`${apiPrefix}/${encodeURIComponent(projectId)}/runs`, { signal })
    return response.items ?? []
  },
  async getWorkspace(projectId: string, runId: string, signal?: AbortSignal): Promise<ControlledExecutionWorkspace> {
    const path = `${apiPrefix}/${encodeURIComponent(projectId)}/runs/${encodeURIComponent(runId)}`
    const runPromise = request<BrowserRpaRun>(path, { signal })
    const [run, steps, events, evidence, session] = await Promise.all([
      runPromise,
      request<{ items?: BrowserRpaRunStep[] }>(`${path}/steps`, { signal }),
      request<{ items?: BrowserRpaRunEvent[] }>(`${path}/events`, { signal }),
      request<{ items?: BrowserRpaEvidence[] }>(`${path}/evidence`, { signal }),
      runPromise.then(run => Promise.all([
        request<BrowserRpaEnvironment>(`${apiPrefix}/${encodeURIComponent(projectId)}/environments/${encodeURIComponent(run.environment_id)}`, { signal }),
        request<BrowserRpaProfile>(`${apiPrefix}/${encodeURIComponent(projectId)}/browser-profiles/${encodeURIComponent(run.profile_id)}`, { signal }),
        request<BrowserRpaSitePolicy>(`${apiPrefix}/${encodeURIComponent(projectId)}/site-policies/${encodeURIComponent(run.policy_id)}`, { signal }),
        run.lease_id
          ? request<BrowserRpaLease>(`${path}/leases/${encodeURIComponent(run.lease_id)}`, { signal }).catch(error => {
              if (error instanceof ControlledExecutionApiError && (error.status === 403 || error.status === 404)) return undefined
              throw error
            })
          : Promise.resolve(undefined),
      ])),
    ])
    const [environment, profile, policy, lease] = session
    return { run, steps: steps.items ?? [], events: events.items ?? [], evidence: evidence.items ?? [], environment, profile, policy, lease }
  },

  generatePlan(projectId: string, runId: string) {
    return request<RunnerV3Plan>(runPath(projectId, runId, ':plan'), { method: 'POST' })
  },
  checkSession(projectId: string, runId: string) {
    return request<EdgeSessionProbe>(runPath(projectId, runId, ':check-session'), { method: 'POST' })
  },
  acquireLease(projectId: string, runId: string, expectedVersion: number) {
    return request<{ run: BrowserRpaRun; lease: BrowserRpaLease }>(`${runPath(projectId, runId)}/leases`, {
      method: 'POST', body: JSON.stringify({ expected_version: expectedVersion }),
    })
  },
  heartbeatLease(projectId: string, runId: string, lease: BrowserRpaLease) {
    return request<BrowserRpaLease>(`${runPath(projectId, runId)}/leases/${encodeURIComponent(lease.id)}:heartbeat`, {
      method: 'POST', body: JSON.stringify({ expected_version: lease.version, fencing_token: lease.fencing_token }),
    })
  },
  prepare(projectId: string, runId: string) {
    return request<BrowserRpaRun>(runPath(projectId, runId, ':prepare'), { method: 'POST' })
  },
  reconcileResult(projectId: string, runId: string) {
    return request<BrowserRpaRun>(runPath(projectId, runId, ':reconcile-result'), { method: 'POST' })
  },
  confirm(projectId: string, run: BrowserRpaRun) {
    return request<IssuedFinalConfirmation>(`${runPath(projectId, run.id)}/confirmations`, {
      method: 'POST',
      body: JSON.stringify({ expected_version: run.version, binding_hash: run.authority.approval_action_hash }),
    })
  },
  submit(projectId: string, run: BrowserRpaRun, lease: BrowserRpaLease, confirmation: IssuedFinalConfirmation) {
    return request<BrowserRpaRun>(runPath(projectId, run.id, ':submit'), {
      method: 'POST',
      body: JSON.stringify({
        step_id: `${run.id}-submit-v${run.version}`, confirmation_id: confirmation.confirmation.id,
        token: confirmation.token, lease_id: lease.id, fencing_token: lease.fencing_token,
        idempotency_key: `${run.id}-submit-v${run.version}`,
      }),
    })
  },

  pause(projectId: string, runId: string, expectedVersion: number) {
    return control(projectId, runId, expectedVersion, 'pause')
  },
  resume(projectId: string, runId: string, expectedVersion: number) {
    return control(projectId, runId, expectedVersion, 'resume')
  },
  cancel(projectId: string, runId: string, expectedVersion: number) {
    return control(projectId, runId, expectedVersion, 'cancel')
  },
  takeOver(projectId: string, runId: string, expectedVersion: number) {
    return control(projectId, runId, expectedVersion, 'takeover')
  },
  releaseTakeover(projectId: string, runId: string, expectedVersion: number) {
    return control(projectId, runId, expectedVersion, 'release_takeover')
  },
}

function runPath(projectId: string, runId: string, suffix = '') {
  return `${apiPrefix}/${encodeURIComponent(projectId)}/runs/${encodeURIComponent(runId)}${suffix}`
}

function control(projectId: string, runId: string, expectedVersion: number, action: 'pause' | 'resume' | 'cancel' | 'takeover' | 'release_takeover') {
  return request<BrowserRpaRun>(`${apiPrefix}/${encodeURIComponent(projectId)}/runs/${encodeURIComponent(runId)}:${action}`, {
    method: 'POST',
    body: JSON.stringify({ expected_version: expectedVersion }),
  })
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { credentials: 'include', ...init, headers })
  const payload = await response.json().catch(() => undefined) as T | { error?: string | { message?: string }; message?: string } | undefined
  if (!response.ok) {
    const errorValue = payload && typeof payload === 'object' && 'error' in payload ? payload.error : undefined
    const message = typeof errorValue === 'string'
      ? errorValue
      : errorValue && typeof errorValue === 'object' && typeof errorValue.message === 'string'
        ? errorValue.message
        : payload && typeof payload === 'object' && 'message' in payload && typeof payload.message === 'string'
          ? payload.message
          : '受控执行控制面请求失败'
    throw new ControlledExecutionApiError(response.status, message)
  }
  return payload as T
}
