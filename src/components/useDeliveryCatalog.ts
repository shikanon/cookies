import { useCallback } from 'react'
import { api, type ApiConnectorAccount, type ApiConnectorPlatformObjectKind, type ApiOptimizationTargetContext } from '../data/api'
import type { PlatformObjectLoader, PlatformObjectSort, DouyinVideoLoader } from './DeliveryObjectPickers'

export function useDeliveryCatalog(projectId: string, accountID: string | undefined, connectorAccounts: ApiConnectorAccount[]) {
  const loadPlatformObjectPage = useCallback(async (objectKind: ApiConnectorPlatformObjectKind, query: string, cursor: string | undefined, sortBy: PlatformObjectSort, sortOrder: 'asc' | 'desc', iesCoreUserID?: string) => {
    if (!accountID || !connectorAccounts.some(account => account.id === accountID)) throw new Error('计划没有绑定当前 Project 的已验证巨量账户。')
    return api.listProjectConnectorPlatformObjects(projectId, accountID, { objectKind, iesCoreUserID, status: 'active', q: query || undefined, cursor, limit: 60, sortBy, sortOrder })
  }, [connectorAccounts, accountID, projectId])

  const loadDouyinVideos = useCallback<DouyinVideoLoader>((iesCoreUserID, query, cursor, sortBy, sortOrder) => {
    if (iesCoreUserID && !/^\d+$/.test(iesCoreUserID)) return Promise.reject(new Error('抖音号 ID 必须是数字。'))
    return loadPlatformObjectPage('douyin_video', query, cursor, sortBy, sortOrder, iesCoreUserID || undefined)
  }, [loadPlatformObjectPage])
  const loadVideos = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('video_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadImages = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('image_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadProductImages = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('product_image', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadPhotos = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('aweme_photo_material', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadProducts = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('marketing_product', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadApplications = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('application', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadOptimizationTargets = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('optimization_target', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadOptimizationCapabilities = useCallback((selectedAccountID: string, context: ApiOptimizationTargetContext) => {
    if (!connectorAccounts.some(account => account.id === selectedAccountID)) return Promise.reject(new Error('请选择当前 Project 已验证账户。'))
    return api.readProjectOptimizationTargetCapabilities(projectId, selectedAccountID, context)
  }, [projectId, connectorAccounts])
  const loadAuthorizedIdentities = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('authorized_identity', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadCategories = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('industry_category', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  const loadBrands = useCallback<PlatformObjectLoader>((query, cursor, sortBy, sortOrder) => loadPlatformObjectPage('brand', query, cursor, sortBy, sortOrder), [loadPlatformObjectPage])
  return { loadDouyinVideos, loadVideos, loadImages, loadProductImages, loadPhotos, loadProducts, loadApplications, loadOptimizationTargets, loadOptimizationCapabilities, loadAuthorizedIdentities, loadCategories, loadBrands }
}
