import type { ProductionCenterGateway } from './gateway'
import type { ProductionAssetPage, ProductionAssetQuery, ProductionRunDetail, ProductionRunPage, ProductionRunQuery, ProductionRunRef, RetryResult } from './types'

type Fetcher = typeof fetch
const browserFetch: Fetcher = (input, init) => fetch(input, init)

export class ProductionGatewayError extends Error {
  constructor(message: string, readonly code: string, readonly status: number) {
    super(message)
    this.name = 'ProductionGatewayError'
  }
}

export class HttpProductionCenterGateway implements ProductionCenterGateway {
  private readonly active = new Map<'runs' | 'detail' | 'assets', AbortController>()

  constructor(
    private readonly fetcher: Fetcher = browserFetch,
    private readonly origin: string = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? '',
  ) {}

  listRuns(projectId: string, query: ProductionRunQuery) {
    return this.request<ProductionRunPage>('runs', this.path(projectId, 'production-runs', query))
  }

  getRun(projectId: string, ref: ProductionRunRef) {
    const path = `production-runs/${encodeURIComponent(ref.source)}/${encodeURIComponent(ref.id)}`
    return this.request<ProductionRunDetail>('detail', this.path(projectId, path))
  }

  retryRun(projectId: string, ref: ProductionRunRef, idempotencyKey: string) {
    const path = `production-runs/${encodeURIComponent(ref.source)}/${encodeURIComponent(ref.id)}:retry`
    return this.request<RetryResult>('detail', this.path(projectId, path), {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
    })
  }

  listAssets(projectId: string, query: ProductionAssetQuery) {
    return this.request<ProductionAssetPage>('assets', this.path(projectId, 'production-assets', query))
  }

  private path(projectId: string, resource: string, query?: ProductionRunQuery | ProductionAssetQuery) {
    const url = new URL(`${this.origin}/api/creative/v1/projects/${encodeURIComponent(projectId)}/${resource}`, this.origin || window.location.origin)
    if (query) {
      for (const [key, raw] of Object.entries(query)) {
        if (raw === undefined || raw === '' || Array.isArray(raw) && raw.length === 0) continue
        url.searchParams.set(key, Array.isArray(raw) ? raw.join(',') : String(raw))
      }
    }
    return url.toString()
  }

  private async request<T>(slot: 'runs' | 'detail' | 'assets', url: string, init: RequestInit = {}): Promise<T> {
    this.active.get(slot)?.abort()
    const controller = new AbortController()
    this.active.set(slot, controller)
    try {
      const response = await this.fetcher(url, { ...init, credentials: 'include', signal: controller.signal })
      const text = await response.text()
      const payload = text ? JSON.parse(text) as T | { code?: string; message?: string; error?: { code?: string; message?: string } } : {}
      if (!response.ok) {
		const problem = payload as { code?: string; message?: string; error?: { code?: string; message?: string } }
		throw new ProductionGatewayError(problem.message ?? problem.error?.message ?? `制作中心请求失败（HTTP ${response.status}）`, problem.code ?? problem.error?.code ?? 'PRODUCTION_REQUEST_FAILED', response.status)
      }
      return payload as T
    } finally {
      if (this.active.get(slot) === controller) this.active.delete(slot)
    }
  }
}
