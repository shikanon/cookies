import type { ProductionAssetPage, ProductionAssetQuery, ProductionRunDetail, ProductionRunPage, ProductionRunQuery, ProductionRunRef, RetryResult } from './types'

export interface ProductionCenterGateway {
  listRuns(projectId: string, query: ProductionRunQuery): Promise<ProductionRunPage>
  getRun(projectId: string, ref: ProductionRunRef): Promise<ProductionRunDetail>
  retryRun(projectId: string, ref: ProductionRunRef, idempotencyKey: string): Promise<RetryResult>
  listAssets(projectId: string, query: ProductionAssetQuery): Promise<ProductionAssetPage>
}
