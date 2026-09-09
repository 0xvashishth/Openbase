// Shared transport: one fetch wrapper for the whole SDK.
// - JSON encode/decode, typed `{ data, error }` results (never throws on HTTP errors)
// - apikey + Bearer attach, Prefer/Range passthrough, x-request-id echo
// - retry once on 502/503/504 with jitter, AbortController timeout

import { OpenbaseError, parseErrorBody, type PostgrestErrorShape } from './errors.js'

export type FetchImpl = typeof fetch

export interface RequestOptions {
  method?: string
  headers?: Record<string, string>
  body?: unknown // object → JSON; string/Uint8Array/FormData passed through
  query?: Record<string, string | number | boolean | undefined | null>
  timeoutMs?: number
  retry?: boolean
  fetch?: FetchImpl
}

export interface Ok<T> {
  data: T
  error: null
  status: number
  statusText: string
  count?: number | null
}

export interface Err {
  data: null
  error: PostgrestErrorShape
  status: number
  statusText: string
  count?: null
}

export type Result<T> = Ok<T> | Err

export interface TransportConfig {
  baseUrl: string
  apikey: string
  accessToken?: () => string | null | Promise<string | null>
  globalHeaders?: Record<string, string>
  fetch?: FetchImpl
  timeoutMs?: number
}

const RETRYABLE = new Set([502, 503, 504])

function resolveFetch(custom?: FetchImpl): FetchImpl {
  if (custom) return custom
  if (typeof globalThis.fetch === 'function') return globalThis.fetch.bind(globalThis)
  throw new OpenbaseError('No fetch implementation available. Pass one via createClient(..., { global: { fetch } })', {
    code: 'validation',
  })
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

/** Parse `Content-Range: 0-19/42` → 42 (or null for `*`). */
export function parseCountFromContentRange(header: string | null): number | null {
  if (!header) return null
  const parts = header.split('/')
  if (parts.length !== 2) return null
  const total = parts[1]?.trim()
  if (!total || total === '*') return null
  const n = Number(total)
  return Number.isFinite(n) ? n : null
}

export class Transport {
  readonly baseUrl: string
  readonly apikey: string
  private accessToken?: () => string | null | Promise<string | null>
  private globalHeaders: Record<string, string>
  private customFetch?: FetchImpl
  private defaultTimeoutMs: number

  constructor(cfg: TransportConfig) {
    this.baseUrl = cfg.baseUrl
    this.apikey = cfg.apikey
    this.accessToken = cfg.accessToken
    this.globalHeaders = cfg.globalHeaders ?? {}
    this.customFetch = cfg.fetch
    this.defaultTimeoutMs = cfg.timeoutMs ?? 30_000
  }

  setAccessToken(fn: () => string | null | Promise<string | null>): void {
    this.accessToken = fn
  }

  async request<T>(path: string, opts: RequestOptions = {}): Promise<Result<T>> {
    const fetchImpl = resolveFetch(opts.fetch ?? this.customFetch)
    const url = new URL(path, this.baseUrl + '/')
    if (opts.query) {
      for (const [k, v] of Object.entries(opts.query)) {
        if (v === undefined || v === null) continue
        url.searchParams.set(k, String(v))
      }
    }

    const buildHeaders = async (): Promise<Record<string, string>> => {
      const h: Record<string, string> = { ...this.globalHeaders, ...(opts.headers ?? {}) }
      if (!h['apikey']) h['apikey'] = this.apikey
      const token = this.accessToken ? await this.accessToken() : null
      if (token && !h['Authorization']) h['Authorization'] = `Bearer ${token}`
      else if (!h['Authorization']) h['Authorization'] = `Bearer ${this.apikey}`
      return h
    }

    const isJsonBody =
      opts.body !== undefined &&
      typeof opts.body === 'object' &&
      !(opts.body instanceof FormData) &&
      !(opts.body instanceof Uint8Array) &&
      !(opts.body instanceof ArrayBuffer)

    const attempt = async (): Promise<Response> => {
      const headers = await buildHeaders()
      if (isJsonBody && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
      const controller = new AbortController()
      const timeoutMs = opts.timeoutMs ?? this.defaultTimeoutMs
      const timer = timeoutMs > 0 ? setTimeout(() => controller.abort(), timeoutMs) : null
      try {
        return await fetchImpl(url.toString(), {
          method: opts.method ?? 'GET',
          headers,
          body:
            opts.body === undefined
              ? undefined
              : isJsonBody
                ? JSON.stringify(opts.body)
                : (opts.body as BodyInit),
          signal: controller.signal,
        })
      } finally {
        if (timer) clearTimeout(timer)
      }
    }

    let res: Response
    try {
      res = await attempt()
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') {
        return {
          data: null,
          error: { message: 'Request timed out', code: 'timeout', details: null, hint: null },
          status: 0,
          statusText: '',
          count: null,
        }
      }
      return {
        data: null,
        error: { message: e instanceof Error ? e.message : 'Network request failed', code: 'network', details: null, hint: null },
        status: 0,
        statusText: '',
        count: null,
      }
    }

    // Retry once on 502/503/504 with jitter (idempotent GETs + explicit opt-in).
    if (RETRYABLE.has(res.status) && (opts.retry ?? (opts.method ?? 'GET') === 'GET')) {
      await sleep(150 + Math.floor(Math.random() * 250))
      try {
        res = await attempt()
      } catch (e) {
        return {
          data: null,
          error: { message: e instanceof Error ? e.message : 'Network request failed', code: 'network', details: null, hint: null },
          status: 0,
          statusText: '',
          count: null,
        }
      }
    }

    const statusText = (res as { statusText?: string }).statusText ?? ''
    const count = parseCountFromContentRange(res.headers.get('content-range'))
    const text = await res.text().catch(() => '')
    let json: unknown = null
    if (text) {
      try {
        json = JSON.parse(text)
      } catch {
        json = null
      }
    }

    if (!res.ok) {
      return { data: null, error: parseErrorBody(json, res.status), status: res.status, statusText, count: null }
    }
    if (json === null) {
      // 204 No Content / empty body with return=minimal.
      return { data: null as unknown as T, error: null, status: res.status, statusText, count }
    }
    return { data: json as T, error: null, status: res.status, statusText, count }
  }
}
