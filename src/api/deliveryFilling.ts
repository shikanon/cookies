import { DeliveryApiError, type StableReference } from './delivery'
import type { OceanConfiguration } from '../lib/deliveryChoices'

export type FillingStrategy = { package_id: string; version: number; content_hash: string }
export type FillingValues = {
  search_expansion: boolean; search_keywords: string[]; search_terms: string[]; regions: string[]; age_ranges: string[]; gender: string; smart_expansion: boolean;
  source_label: string; call_to_action: string[]; category: StableReference | undefined; product_images: StableReference[];
  name: string; objective: string; account: StableReference; marketing_purpose: string;
  product: StableReference | undefined; materials: StableReference[]; carrier: string;
  optimization_target: StableReference | undefined; product_name: string; selling_points: string[];
  copy: string; total_budget: number; daily_budget: number; bid: number | undefined; project_bid: number | undefined;
  schedule: OceanConfiguration['project']['schedule']; landing_page: StableReference | undefined;
  monitoring: StableReference[]; identity: StableReference | undefined;
}
export type FillingField = keyof FillingValues
export type FillingSuggestion = { [K in FillingField]: { field: K; value: FillingValues[K]; reason: string; sources: string[] } }[FillingField]
export type FillingRequest = {
  page: 'plan' | 'configuration'; plan_id?: string; target_id?: string; ocean: OceanConfiguration;
  current: Partial<FillingValues>; fields: FillingField[]; strategy?: FillingStrategy; search?: string; instructions?: string; material_count?: number; copy_count?: number;
  finance?: { unit_weight: number };
}
export type FillingFinance = { rule_version: string; entries: Array<{ field: 'daily_budget' | 'bid' | 'project_bid'; amount_minor: number; minimum_minor: number; maximum_minor: number; basis: string; sources: string[] }>; warnings: string[] }
export type FillingResult = { suggestions: FillingSuggestion[]; warnings: string[]; context_hash: string; strategy?: FillingStrategy; provider: string; model: string; route_revision?: string; finance?: FillingFinance }
export type FillingAcceptance = { result: FillingResult; target_id?: string; accepted_at: string }

async function request<T>(url: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal, method: body ? 'POST' : 'GET', credentials: 'include', headers: body ? { 'Content-Type': 'application/json' } : undefined, body: body ? JSON.stringify(body) : undefined })
  const value = await response.json()
  if (!response.ok) throw new DeliveryApiError(value.error?.code, response.status, value.error?.message || '读取智能填写数据失败。')
  return value as T
}
export const deliveryFillingApi = {
  suggest: (projectId: string, body: FillingRequest, signal?: AbortSignal) => request<FillingResult>(`/api/delivery/v1/projects/${encodeURIComponent(projectId)}/filling-suggestions`, body, signal),
  strategies: async (projectId: string) => {
    const value = await request<{ items: Array<FillingStrategy & { status: string; snapshot: { strategy: { objective: string }; approval: { approved_by: string } } }> }>(`/api/strategy/v1/projects/${encodeURIComponent(projectId)}/strategy-packages`)
    return value.items.filter(item => item.status === 'published' && item.snapshot.approval.approved_by).map(item => ({ package_id: item.package_id, version: item.version, content_hash: item.content_hash, label: `${item.snapshot.strategy.objective || item.package_id} · V${item.version}` }))
  },
}
