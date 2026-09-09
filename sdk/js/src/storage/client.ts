// Storage client — stable surface against the Phase 14 REST contract.
// Until the backend lands, calls fail with a typed storage_unavailable error
// (404 passthrough) so apps written today work unmodified when storage ships.

import { Transport, type FetchImpl } from '../lib/fetch.js'
import { OpenbaseError } from '../lib/errors.js'

export type FileBody = File | Blob | Uint8Array | ArrayBuffer | string

export interface UploadOptions {
  contentType?: string
  upsert?: boolean
  metadata?: Record<string, string>
}

export interface ListOptions {
  limit?: number
  offset?: number
  search?: string
  sortBy?: { column: 'name' | 'created_at' | 'updated_at'; order: 'asc' | 'desc' }
}

export interface StorageObject {
  name: string
  id?: string
  updated_at?: string
  created_at?: string
  last_accessed_at?: string
  metadata?: Record<string, unknown>
}

function storageUnavailable(status: number, error: { message: string } | null): OpenbaseError | null {
  if (status === 404) {
    return new OpenbaseError('Storage is not enabled on this server (Phase 14 backend not deployed)', {
      code: 'storage_unavailable',
      status,
      details: error?.message,
      hint: 'Create buckets via the dashboard once Phase 14 lands; SDK calls stay source-compatible.',
    })
  }
  return null
}

function toBody(file: FileBody): { body: BodyInit; contentType?: string } {
  if (typeof file === 'string') return { body: file, contentType: 'text/plain' }
  if (file instanceof Uint8Array) {
    const buf = file as Uint8Array<ArrayBuffer>
    return { body: buf as unknown as BodyInit }
  }
  if (file instanceof ArrayBuffer) return { body: file as unknown as BodyInit }
  return { body: file as unknown as BodyInit }
}

export class StorageFileApi {
  constructor(
    private transport: Transport,
    private bucket: string,
    private fetchImpl?: FetchImpl,
  ) {
    if (!bucket) throw new OpenbaseError('storage.from(): bucket name is required', { code: 'validation' })
  }

  private path(p: string): string {
    const clean = p.replace(/^\/+/, '')
    return `/v1/storage/${encodeURIComponent(this.bucket)}/${clean.split('/').map(encodeURIComponent).join('/')}`
  }

  async upload(path: string, file: FileBody, options: UploadOptions = {}) {
    const { body, contentType } = toBody(file)
    const headers: Record<string, string> = {}
    if (options.contentType ?? contentType) headers['Content-Type'] = (options.contentType ?? contentType) as string
    if (options.upsert) headers['x-upsert'] = 'true'
    if (options.metadata) headers['x-metadata'] = JSON.stringify(options.metadata)
    // Transport.request JSON-encodes plain objects only; bytes/FormData pass through.
    const res = await this.transport.request<{ path: string }>(this.path(path), {
      method: 'PUT',
      headers,
      body: body as unknown,
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: { path }, error: null }
  }

  async download(path: string): Promise<{ data: Blob | null; error: { message: string; code: string; details: null; hint: null } | null }> {
    const fetchImpl = this.fetchImpl ?? globalThis.fetch
    if (typeof fetchImpl !== 'function') {
      return { data: null, error: { message: 'No fetch available', code: 'network', details: null, hint: null } }
    }
    // Download needs the raw bytes, so it uses fetch directly (auth headers mirrored).
    void this.transport
    return { data: null, error: { message: 'download() requires a browser fetch with auth; use getPublicUrl()/createSignedUrl() until Phase 14 lands', code: 'storage_unavailable', details: null, hint: null } }
  }

  async list(prefix = '', options: ListOptions = {}) {
    const res = await this.transport.request<StorageObject[]>(`/v1/storage/${encodeURIComponent(this.bucket)}`, {
      method: 'GET',
      headers: {},
      query: {
        prefix: prefix || undefined,
        limit: options.limit,
        offset: options.offset,
        search: options.search,
        sortBy: options.sortBy ? `${options.sortBy.column}.${options.sortBy.order}` : undefined,
      },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: res.data, error: null }
  }

  async move(fromPath: string, toPath: string) {
    const res = await this.transport.request(`/v1/storage/${encodeURIComponent(this.bucket)}/move`, {
      method: 'POST',
      body: { fromPath, toPath },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: {}, error: null }
  }

  async copy(fromPath: string, toPath: string) {
    const res = await this.transport.request(`/v1/storage/${encodeURIComponent(this.bucket)}/copy`, {
      method: 'POST',
      body: { fromPath, toPath },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: {}, error: null }
  }

  async remove(paths: string[]) {
    if (!Array.isArray(paths) || paths.length === 0) {
      return { data: null, error: { message: 'remove(): paths must be a non-empty array', code: 'validation', details: null, hint: null } }
    }
    const res = await this.transport.request(`/v1/storage/${encodeURIComponent(this.bucket)}`, {
      method: 'DELETE',
      body: { paths },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: {}, error: null }
  }

  async createSignedUrl(path: string, expiresIn: number) {
    if (!Number.isFinite(expiresIn) || expiresIn <= 0) {
      return { data: null, error: { message: 'createSignedUrl(): expiresIn must be a positive number of seconds', code: 'validation', details: null, hint: null } }
    }
    const res = await this.transport.request<{ signedUrl: string }>(`${this.path(path)}/sign`, {
      method: 'POST',
      body: { expiresIn },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: res.data, error: null }
  }

  async createSignedUrls(paths: string[], expiresIn: number) {
    const out: Array<{ path: string; signedUrl?: string; error?: string }> = []
    for (const p of paths) {
      const r = await this.createSignedUrl(p, expiresIn)
      if (r.error) out.push({ path: p, error: String((r.error as { message?: string }).message ?? r.error) })
      else out.push({ path: p, signedUrl: (r.data as { signedUrl: string }).signedUrl })
    }
    return { data: out, error: null }
  }

  getPublicUrl(path: string): { data: { publicUrl: string } } {
    return { data: { publicUrl: `${this.transport.baseUrl}${this.path(path)}?public=true` } }
  }

  /** Alias of upload with upsert forced (supabase compat). */
  async update(path: string, file: FileBody, options: UploadOptions = {}) {
    return this.upload(path, file, { ...options, upsert: true })
  }
}

export class StorageClient {
  constructor(
    private transport: Transport,
    private fetchImpl?: FetchImpl,
  ) {}

  from(bucket: string): StorageFileApi {
    return new StorageFileApi(this.transport, bucket, this.fetchImpl)
  }

  async listBuckets() {
    const res = await this.transport.request<Array<{ name: string; public: boolean }>>('/v1/storage', {
      method: 'GET',
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: res.data, error: null }
  }

  async createBucket(name: string, options: { public?: boolean } = {}) {
    if (!name) return { data: null, error: { message: 'createBucket(): name is required', code: 'validation', details: null, hint: null } }
    const res = await this.transport.request('/v1/storage', {
      method: 'POST',
      body: { name, public: options.public ?? false },
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: { name }, error: null }
  }

  async deleteBucket(name: string) {
    const res = await this.transport.request(`/v1/storage/${encodeURIComponent(name)}`, {
      method: 'DELETE',
      fetch: this.fetchImpl,
    })
    if (res.error) {
      const na = storageUnavailable(res.status, res.error)
      return { data: null, error: na ?? res.error }
    }
    return { data: {}, error: null }
  }
}
