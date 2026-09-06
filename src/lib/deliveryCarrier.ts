type CarrierProject = { carrier: string }
type LandingPageReference = { namespace?: string; object_kind?: string; id?: string }
type CarrierPromotion = { landing_page_reference?: LandingPageReference }

export function carrierUsesOrangeLandingPage(carrier: string) {
  return carrier === 'orange_landing_page' || carrier === 'orange_landing_page_and_im'
}

export function changeOceanEngineCarrier<
  T extends { project: CarrierProject; promotions: CarrierPromotion[] },
>(configuration: T, carrier: string): T {
  if (carrier === configuration.project.carrier) return configuration
  return {
    ...configuration,
    project: { ...configuration.project, carrier },
    promotions: configuration.promotions.map(promotion => ({ ...promotion, landing_page_reference: undefined })),
  }
}

export function normalizeOceanEngineLandingPages<
  T extends { project: CarrierProject; promotions: CarrierPromotion[] },
>(configuration: T): T {
  const carrier = configuration.project.carrier
  let changed = false
  const promotions = configuration.promotions.map(promotion => {
    const reference = promotion.landing_page_reference
    if (!reference) return promotion
    if (carrier === 'owned_landing_page') {
      if (reference.object_kind === 'orange_landing_page') {
        changed = true
        return { ...promotion, landing_page_reference: undefined }
      }
      if (reference.object_kind === 'landing_page' && reference.id?.startsWith('http')) {
        changed = true
        return {
          ...promotion,
          landing_page_reference: { ...reference, namespace: 'cookies', object_kind: 'owned_landing_page' },
        }
      }
      return promotion
    }
    if (carrierUsesOrangeLandingPage(carrier)) {
      if (reference.object_kind !== 'owned_landing_page') return promotion
      changed = true
      return { ...promotion, landing_page_reference: undefined }
    }
    changed = true
    return { ...promotion, landing_page_reference: undefined }
  })
  return changed ? { ...configuration, promotions } : configuration
}
