// URL / key validation + shared helpers.

import { OpenbaseError } from './errors.js'

export function stripTrailingSlash(url: string): string {
  return url.replace(/\/+$/, '')
}

/** Validate the base URL once at createClient time with actionable errors. */
export function normalizeBaseUrl(raw: string): string {
  if (!raw || typeof raw !== 'string' || raw.trim() === '') {
    throw new OpenbaseError('createClient: url is required (e.g. http://localhost:8080)', { code: 'validation' })
  }
  const trimmed = stripTrailingSlash(raw.trim())
  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    throw new OpenbaseError(
      `createClient: url is not a valid URL: ${JSON.stringify(raw)}. Expected e.g. http://localhost:8080`,
      { code: 'validation' },
    )
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new OpenbaseError(`createClient: url must start with http(s)://, got ${JSON.stringify(raw)}`, {
      code: 'validation',
    })
  }
  return trimmed
}

/**
 * Validate the anon/service key. `ob_` keys are accepted;JWT-shaped project
 * tokens (Phase 10 end-user JWTs) are passed through untouched.
 */
export function normalizeKey(raw: string): string {
  if (!raw || typeof raw !== 'string' || raw.trim() === '') {
    throw new OpenbaseError('createClient: key is required (anon public key, e.g. ob_…)', { code: 'validation' })
  }
  const key = raw.trim()
  if (key.startsWith('ob_')) {
    if (key.length < 8) {
      throw new OpenbaseError('createClient: key looks truncated (ob_… keys are longer than this)', {
        code: 'validation',
      })
    }
    return key
  }
  // Allow JWTs / future key formats through — the server is authoritative.
  if (key.split('.').length === 3) return key
  throw new OpenbaseError(
    'createClient: key must be an ob_… anon key (or a JWT end-user token). ' +
      'Never use service_role keys in a browser bundle.',
    { code: 'validation' },
  )
}

/** Guard: service_role keys must never ship to a browser. */
export function assertNotServiceRoleInBrowser(key: string, method: string): void {
  if (typeof window !== 'undefined' && key.startsWith('ob_service_')) {
    throw new OpenbaseError(
      `${method}: service_role keys must never be used in a browser. ` +
        'Use the anon key in the browser and service_role only on your server.',
      { code: 'validation' },
    )
  }
}

/** ws(s) URL derived from the http(s) base URL. */
export function realtimeUrl(baseUrl: string): string {
  const u = new URL(stripTrailingSlash(baseUrl))
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  u.pathname = u.pathname.replace(/\/+$/, '') + '/v1/realtime'
  u.search = ''
  u.hash = ''
  return u.toString()
}

/** Build a URL with query params, skipping undefined/null. */
export function withQuery(base: string, params: Record<string, string | number | boolean | undefined | null>): string {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null) continue
    qs.set(k, String(v))
  }
  const s = qs.toString()
  return s ? `${base}?${s}` : base
}

/** Single-flight helper for token refresh (concurrent callers share one promise). */
export function singleflight<T>(): {
  run(fn: () => Promise<T>): Promise<T>
  get pending(): boolean
} {
  let current: Promise<T> | null = null
  return {
    get pending() {
      return current !== null
    },
    run(fn: () => Promise<T>): Promise<T> {
      if (current) return current
      current = fn().finally(() => {
        current = null
      })
      return current
    },
  }
}
