// OpenbaseClient — composition root: auth + database + realtime + storage +
// functions over one transport. Mirrors the supabase-js surface
// (`createClient(url, key)` → `.auth`, `.from()`, `.rpc()`, `.channel()`,
// `.storage`, `.functions`) while staying honest about backend capabilities.

import { Transport, type FetchImpl } from './lib/fetch.js'
import type { PostgrestErrorShape } from './lib/errors.js'
import { normalizeBaseUrl, normalizeKey, realtimeUrl } from './lib/helpers.js'
import { defaultStorage, type SessionStorageAdapter } from './lib/storage.js'
import { AuthClient } from './auth/client.js'
import { PostgrestClient, PostgrestQueryBuilder } from './postgrest/builder.js'
import { RealtimeClient, RealtimeChannel, type WebSocketFactory } from './realtime/channel.js'
import { StorageClient } from './storage/client.js'
import { FunctionsClient } from './functions/client.js'

export interface OpenbaseClientOptions {
  auth?: {
    storage?: SessionStorageAdapter
    storageKey?: string
    persistSession?: boolean
    autoRefreshToken?: boolean
    detectSessionInUrl?: boolean
    flowType?: 'pkce' | 'implicit'
  }
  db?: { schema?: string }
  realtime?: {
    params?: Record<string, string>
    wsFactory?: WebSocketFactory
    createTicket?: () => Promise<string | null> | string | null
    reconnectMs?: number
  }
  global?: {
    headers?: Record<string, string>
    fetch?: FetchImpl
  }
}

export class OpenbaseClient {
  readonly baseUrl: string
  readonly apikey: string
  readonly auth: AuthClient
  readonly storage: StorageClient
  readonly functions: FunctionsClient
  private transport: Transport
  private postgrest: PostgrestClient
  private realtimeClient: RealtimeClient | null = null
  private realtimeOpts?: OpenbaseClientOptions['realtime']
  private fetchImpl?: FetchImpl

  constructor(baseUrl: string, apikey: string, options: OpenbaseClientOptions = {}) {
    this.baseUrl = normalizeBaseUrl(baseUrl)
    this.apikey = normalizeKey(apikey)
    this.fetchImpl = options.global?.fetch
    this.transport = new Transport({
      baseUrl: this.baseUrl,
      apikey: this.apikey,
      accessToken: () => null, // rewired below once auth exists
      globalHeaders: options.global?.headers,
      fetch: this.fetchImpl,
    })
    this.auth = new AuthClient(this.baseUrl, this.apikey, {
      storage: options.auth?.storage ?? defaultStorage(),
      storageKey: options.auth?.storageKey,
      persistSession: options.auth?.persistSession,
      autoRefreshToken: options.auth?.autoRefreshToken,
      detectSessionInUrl: options.auth?.detectSessionInUrl,
      flowType: options.auth?.flowType,
      fetch: this.fetchImpl,
    })
    // End-user JWT (when signed in) takes precedence over the anon key.
    this.transport.setAccessToken(async () => {
      const { data } = await this.auth.getSession()
      return data.session?.access_token ?? null
    })
    this.postgrest = new PostgrestClient(this.transport, this.fetchImpl)
    this.storage = new StorageClient(this.transport, this.fetchImpl)
    this.functions = new FunctionsClient(this.transport, this.fetchImpl)
    this.realtimeOpts = options.realtime
    void options.db?.schema // reserved: PostgREST schema scoping (Phase 12)
  }

  from<T = Record<string, unknown>>(table: string): PostgrestQueryBuilder<T> {
    return this.postgrest.from<T>(table)
  }

  rpc<T = unknown>(fn: string, args?: Record<string, unknown>, options?: { count?: 'exact' | 'planned' | 'estimated' | null }) {
    return this.postgrest.rpc<T>(fn, args, options)
  }

  channel(topic: string, opts?: { config?: { broadcast?: { ack?: boolean }; presence?: { key?: string } } }): RealtimeChannel {
    if (!this.realtimeClient) {
      this.realtimeClient = new RealtimeClient(realtimeUrl(this.baseUrl), {
        apikey: this.apikey,
        wsFactory: this.realtimeOpts?.wsFactory,
        reconnectMs: this.realtimeOpts?.reconnectMs,
        createTicket: this.realtimeOpts?.createTicket,
        params: this.realtimeOpts?.params,
      })
    }
    return this.realtimeClient.channel(topic, opts)
  }

  getChannels(): RealtimeChannel[] {
    return this.realtimeClient?.getChannels() ?? []
  }

  removeChannel(ch: RealtimeChannel) {
    return this.realtimeClient?.removeChannel(ch) ?? Promise.resolve({ status: 'ok' })
  }

  removeAllChannels() {
    return this.realtimeClient?.removeAllChannels() ?? Promise.resolve({ status: 'ok' })
  }

  /** Live tables reachable with the current key (today's backend). */
  async getTables(): Promise<{ data: Array<{ name: string }> | null; error: PostgrestErrorShape | null }> {
    const res = await this.transport.request<Array<{ name: string }>>('/v1/api/tables', {
      method: 'GET',
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: null, error: res.error }
    return { data: res.data, error: null }
  }

  /** Column metadata for one collection (today's backend). */
  async getSchema(collection: string) {
    const res = await this.transport.request(`/v1/api/${encodeURIComponent(collection)}/_schema`, {
      method: 'GET',
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: null, error: res.error }
    return { data: res.data, error: null }
  }

  /** Liveness probe (unauthenticated by design). */
  async serverVersion(): Promise<{ data: { status: string } | null; error: PostgrestErrorShape | null }> {
    const res = await this.transport.request<{ status: string }>('/healthz', { method: 'GET', fetch: this.fetchImpl })
    if (res.error) return { data: null, error: res.error }
    return { data: res.data, error: null }
  }
}

export function createClient(url: string, key: string, options: OpenbaseClientOptions = {}): OpenbaseClient {
  return new OpenbaseClient(url, key, options)
}
