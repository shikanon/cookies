export function oceanEngineImageSourceIdentity(value: unknown) {
  if (typeof value !== 'string' || !value.trim()) return ''
  try {
    const parsed = new URL(value.trim(), 'https://cookies.invalid')
    const path = decodeURIComponent(parsed.pathname).replace(/^\/+/, '')
    const identity = path.split('~', 1)[0]
    return isOceanEngineImageSourceIdentity(identity) ? identity : ''
  } catch {
    return ''
  }
}

export function isOceanEngineImageSourceIdentity(value: unknown) {
  if (typeof value !== 'string') return false
  const identity = value.trim()
  return identity.includes('/')
    && !identity.includes('://')
    && !/[?#]/.test(identity)
    && !identity.startsWith('api/')
}
