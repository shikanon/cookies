import catalog from '../../internal/platform/oceanengineconstraints/oceanengine-bid-constraints-v1.json'

export type OceanEngineChargingMode = 'CPC' | 'CPM' | 'OCPC' | 'OCPM'

export type OceanEngineBidConstraint = {
  schemaVersion: string
  chargingMode: OceanEngineChargingMode
  minimumMinor: number
  maximumMinor: number
  maximumSource: 'static' | 'daily_budget'
}

type PricingRule = {
  minimum_minor: number
  maximum_minor: number
  maximum_source: 'static' | 'daily_budget'
}

const pricingRules = catalog.pricing_rules as Record<OceanEngineChargingMode, PricingRule>
const targetChargingModes = catalog.target_charging_modes as Record<string, OceanEngineChargingMode>
const targetDisplayNameChargingModes = catalog.target_display_name_charging_modes as Record<string, OceanEngineChargingMode>

export function normalizeOceanEngineChargingMode(value: string): OceanEngineChargingMode | undefined {
  const normalized = value.trim().toUpperCase()
  return Object.hasOwn(pricingRules, normalized) ? normalized as OceanEngineChargingMode : undefined
}

export function resolveOceanEngineChargingMode(
  target: { semantic_key?: string; display_name_snapshot?: string } | undefined,
  fallback: string,
): OceanEngineChargingMode | undefined {
  const semanticKey = target?.semantic_key?.trim()
  if (semanticKey && targetChargingModes[semanticKey]) return targetChargingModes[semanticKey]
  const displayName = target?.display_name_snapshot?.trim()
  if (displayName && targetDisplayNameChargingModes[displayName]) return targetDisplayNameChargingModes[displayName]
  if (semanticKey || displayName) {
    const fallbackMode = normalizeOceanEngineChargingMode(fallback)
    return fallbackMode === 'OCPC' || fallbackMode === 'OCPM'
      ? fallbackMode
      : catalog.default_target_charging_mode as OceanEngineChargingMode
  }
  return normalizeOceanEngineChargingMode(fallback)
}

export function resolveOceanEngineBidConstraint(
  chargingMode: string,
  dailyBudgetMinor: number,
): OceanEngineBidConstraint | undefined {
  const normalized = normalizeOceanEngineChargingMode(chargingMode)
  if (!normalized) return undefined
  const rule = pricingRules[normalized]
  const maximumMinor = rule.maximum_source === 'daily_budget' && dailyBudgetMinor > 0
    ? Math.min(rule.maximum_minor, dailyBudgetMinor)
    : rule.maximum_minor
  return {
    schemaVersion: catalog.schema_version,
    chargingMode: normalized,
    minimumMinor: rule.minimum_minor,
    maximumMinor,
    maximumSource: rule.maximum_source,
  }
}

export function formatOceanEngineMoneyRange(constraint: OceanEngineBidConstraint) {
  return `${(constraint.minimumMinor / 100).toFixed(2)}～${(constraint.maximumMinor / 100).toFixed(2)} 元`
}
