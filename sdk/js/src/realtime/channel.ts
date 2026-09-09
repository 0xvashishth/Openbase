// Realtime channels. Compatible with today's gateway
// (`{type:subscribe,collection}` → `{type:subscribed|change|error}`,
// `internal/realtime/hub.go`) and forward-compatible with the Phase 16
// surface (postgres_changes event/row filters, broadcast, presence).
//
// postgres_changes event + row filters are enforced client-side so the same
// SDK call is correct before and after the server learns them. Broadcast
// fan-out and presence sync require the Phase 16 backend; until then send()
// transmits the (future) wire message and local presence state is tracked
// in-memory.

export type ChannelBindingType = 'postgres_changes' | 'broadcast' | 'presence'
export type PostgresEvent = 'INSERT' | 'UPDATE' | 'DELETE' | '*'
export type PresenceEvent = 'sync' | 'join' | 'leave' | '*'
export type SubscribeStatus = 'SUBSCRIBED' | 'TIMED_OUT' | 'CHANNEL_ERROR' | 'CLOSED'

export interface PostgresChangesFilter {
  event?: PostgresEvent | string
  table?: string
  collection?: string
  filter?: string // e.g. 'status=eq.open'
  schema?: string
}

export interface BroadcastFilter {
  event?: string
}

export interface PresenceFilter {
  event?: PresenceEvent | string
}

export interface RealtimeChangePayload<T = Record<string, unknown>> {
  event: string
  table: string
  data: T
  old?: T | null
  mode?: 'live' | 'polling'
  lagMs?: number
}

export interface BroadcastPayload<T = unknown> {
  event: string
  payload: T
}

export interface PresencePayload {
  event: string
  key: string
  current?: unknown
  joins?: Record<string, unknown>
  leaves?: Record<string, unknown>
}

/** Minimal WebSocket surface (browser native or `ws` package). */
export interface WebSocketLike {
  readonly readyState: number
  readonly OPEN: number
  send(data: string): void
  close(code?: number, reason?: string): void
  onopen: ((ev: unknown) => void) | null
  onmessage: ((ev: { data: string }) => void) | null
  onclose: ((ev: unknown) => void) | null
  onerror: ((ev: unknown) => void) | null
}

export type WebSocketFactory = (url: string) => WebSocketLike

interface Binding {
  type: ChannelBindingType
  filter: PostgresChangesFilter | BroadcastFilter | PresenceFilter
  callback: (payload: never) => void
}

function normalizeEvent(e: unknown): string {
  return String(e ?? '').toUpperCase()
}

/** Client-side row predicate for `filter: 'col=eq.val'` (subset: eq/neq/gt/gte/lt/lte). */
export function matchesRowFilter(filter: string | undefined, row: Record<string, unknown>): boolean {
  if (!filter) return true
  const m = filter.match(/^([^=]+)=([a-z]+)\.(.*)$/)
  if (!m) return true // unknown grammar: never drop, server is authoritative
  const [, col, op, raw] = m as [string, string, string, string]
  const value = (row as Record<string, unknown>)[col as string]
  switch (op) {
    case 'eq':
      return String(value) === raw
    case 'neq':
      return String(value) !== raw
    case 'gt':
      return Number(value) > Number(raw)
    case 'gte':
      return Number(value) >= Number(raw)
    case 'lt':
      return Number(value) < Number(raw)
    case 'lte':
      return Number(value) <= Number(raw)
    default:
      return true
  }
}

export interface RealtimeClientOptions {
  apikey: string
  wsFactory?: WebSocketFactory
  reconnectMs?: number
  subscribeTimeoutMs?: number
  /** Phase 16 ticket hook: return a single-use ticket to use instead of the apiKey. */
  createTicket?: () => Promise<string | null> | string | null
  params?: Record<string, string>
}

export class RealtimeClient {
  private baseWsUrl: string
  private opts: RealtimeClientOptions
  private socket: WebSocketLike | null = null
  private closed = false
  private connecting = false
  private channels = new Map<string, RealtimeChannel>()
  private statusListeners = new Set<(connected: boolean) => void>()

  constructor(baseWsUrl: string, opts: RealtimeClientOptions) {
    this.baseWsUrl = baseWsUrl
    this.opts = opts
  }

  onStatus(cb: (connected: boolean) => void): () => void {
    this.statusListeners.add(cb)
    return () => this.statusListeners.delete(cb)
  }

  private setStatus(connected: boolean): void {
    for (const cb of this.statusListeners) {
      try {
        cb(connected)
      } catch {
        // ignore
      }
    }
  }

  private async authedUrl(): Promise<string> {
    const ticket = this.opts.createTicket ? await this.opts.createTicket() : null
    const sep = this.baseWsUrl.includes('?') ? '&' : '?'
    if (ticket) return `${this.baseWsUrl}${sep}ticket=${encodeURIComponent(ticket)}`
    let url = `${this.baseWsUrl}${sep}apiKey=${encodeURIComponent(this.opts.apikey)}`
    if (this.opts.params) {
      for (const [k, v] of Object.entries(this.opts.params)) url += `&${encodeURIComponent(k)}=${encodeURIComponent(v)}`
    }
    return url
  }

  get factory(): WebSocketFactory {
    if (this.opts.wsFactory) return this.opts.wsFactory
    const G = globalThis as unknown as { WebSocket?: new (url: string) => WebSocketLike }
    if (typeof G.WebSocket === 'function') {
      return (url) => new G.WebSocket!(url) as unknown as WebSocketLike
    }
    throw new Error('No WebSocket implementation available. Pass wsFactory (e.g. `ws` package) in Node.')
  }

  ensureSocket(): void {
    if (this.closed || this.connecting) return
    if (this.socket && (this.socket.readyState === 1 || this.socket.readyState === 0)) return
    this.connecting = true
    void this.authedUrl().then(
      (url) => {
        if (this.closed) {
          this.connecting = false
          return
        }
        let ws: WebSocketLike
        try {
          ws = this.factory(url)
        } catch {
          this.connecting = false
          return
        }
        this.socket = ws
        ws.onopen = () => {
          this.connecting = false
          this.setStatus(true)
          for (const ch of this.channels.values()) ch.resendSubscriptions()
        }
        ws.onmessage = (ev) => this.route(ev.data)
        ws.onclose = () => {
          this.setStatus(false)
          this.socket = null
          this.connecting = false
          if (!this.closed && this.channels.size > 0) {
            setTimeout(() => this.ensureSocket(), this.opts.reconnectMs ?? 3000)
          } else {
            for (const ch of this.channels.values()) ch.handleSocketClosed()
          }
        }
        ws.onerror = () => {
          this.setStatus(false)
          try {
            ws.close()
          } catch {
            // ignore
          }
        }
      },
      () => {
        this.connecting = false
      },
    )
  }

  sendRaw(obj: unknown): boolean {
    if (this.socket && this.socket.readyState === 1) {
      try {
        this.socket.send(JSON.stringify(obj))
        return true
      } catch {
        return false
      }
    }
    return false
  }

  private route(raw: string): void {
    let msg: Record<string, unknown>
    try {
      msg = JSON.parse(raw) as Record<string, unknown>
    } catch {
      return
    }
    const type = msg['type']
    if (type === 'change') {
      const collection = String(msg['collection'] ?? '')
      const event = normalizeEvent(msg['event'])
      const data = (msg['data'] ?? {}) as Record<string, unknown>
      const mode = msg['mode'] === 'polling' ? 'polling' : 'live'
      for (const ch of this.channels.values()) ch.dispatchChange(collection, event, data, mode, msg['lagMs'] as number | undefined)
      return
    }
    if (type === 'broadcast') {
      for (const ch of this.channels.values()) {
        ch.dispatchBroadcast(String(msg['event'] ?? '*'), msg['payload'])
      }
      return
    }
    if (type === 'presence') {
      for (const ch of this.channels.values()) {
        ch.dispatchPresence(msg as unknown as PresencePayload)
      }
      return
    }
    if (type === 'subscribed') {
      const collection = String(msg['collection'] ?? '')
      for (const ch of this.channels.values()) ch.handleSubscribed(collection)
      return
    }
    if (type === 'error') {
      const collection = msg['collection'] ? String(msg['collection']) : undefined
      const error = String(msg['error'] ?? 'unknown error')
      for (const ch of this.channels.values()) ch.handleChannelError(collection, error)
    }
  }

  channel(topic: string, opts?: { config?: { broadcast?: { ack?: boolean }; presence?: { key?: string } } }): RealtimeChannel {
    let ch = this.channels.get(topic)
    if (!ch) {
      ch = new RealtimeChannel(this, topic, opts?.config)
      this.channels.set(topic, ch)
    }
    return ch
  }

  getChannels(): RealtimeChannel[] {
    return [...this.channels.values()]
  }

  async removeChannel(ch: RealtimeChannel): Promise<{ status: string }> {
    ch.unsubscribe()
    this.channels.delete(ch.topic)
    if (this.channels.size === 0) this.disconnect()
    return { status: 'ok' }
  }

  async removeAllChannels(): Promise<{ status: string }> {
    for (const ch of this.channels.values()) ch.unsubscribe()
    this.channels.clear()
    this.disconnect()
    return { status: 'ok' }
  }

  disconnect(): void {
    this.closed = true
    try {
      this.socket?.close()
    } catch {
      // ignore
    }
    this.socket = null
  }

  /** Test hook: inject a connected fake socket and route a server message. */
  _testInjectMessage(raw: string): void {
    this.route(raw)
  }
}

export class RealtimeChannel {
  readonly topic: string
  readonly config?: { broadcast?: { ack?: boolean }; presence?: { key?: string } }
  private client: RealtimeClient
  private bindings: Binding[] = []
  private subscribed = false
  private subscribeCb: ((status: SubscribeStatus) => void) | null = null
  private subscribedCollections = new Set<string>()
  private errorCb: ((message: string) => void) | null = null
  private localPresence: Record<string, unknown> = {}
  private subscribeTimer: ReturnType<typeof setTimeout> | null = null

  constructor(client: RealtimeClient, topic: string, config?: { broadcast?: { ack?: boolean }; presence?: { key?: string } }) {
    this.client = client
    this.topic = topic
    this.config = config
  }

  on(
    type: 'postgres_changes',
    filter: PostgresChangesFilter,
    callback: (payload: RealtimeChangePayload) => void,
  ): this
  on(type: 'broadcast', filter: BroadcastFilter, callback: (payload: BroadcastPayload) => void): this
  on(type: 'presence', filter: PresenceFilter, callback: (payload: PresencePayload) => void): this
  on(type: ChannelBindingType, filter: PostgresChangesFilter | BroadcastFilter | PresenceFilter, callback: (payload: never) => void): this {
    this.bindings.push({ type, filter, callback })
    return this
  }

  onError(cb: (message: string) => void): this {
    this.errorCb = cb
    return this
  }

  /** Collections required by postgres_changes bindings. */
  private requiredCollections(): string[] {
    const out = new Set<string>()
    for (const b of this.bindings) {
      if (b.type !== 'postgres_changes') continue
      const f = b.filter as PostgresChangesFilter
      const table = f.table ?? f.collection
      if (table) out.add(table)
    }
    return [...out]
  }

  subscribe(cb?: (status: SubscribeStatus) => void): this {
    this.subscribeCb = cb ?? null
    this.client.ensureSocket()
    this.resendSubscriptions()
    if (this.subscribeTimer) clearTimeout(this.subscribeTimer)
    this.subscribeTimer = setTimeout(() => {
      if (!this.subscribed) this.subscribeCb?.('TIMED_OUT')
    }, 10_000)
    const timer = this.subscribeTimer as unknown as { unref?: () => void }
    if (typeof timer.unref === 'function') timer.unref()
    return this
  }

  resendSubscriptions(): void {
    const cols = this.requiredCollections()
    if (cols.length === 0) {
      // Broadcast/presence-only channel: mark subscribed once the socket is up.
      this.subscribed = true
      this.subscribeCb?.('SUBSCRIBED')
      return
    }
    for (const c of cols) this.client.sendRaw({ type: 'subscribe', collection: c })
  }

  unsubscribe(): void {
    for (const c of this.requiredCollections()) this.client.sendRaw({ type: 'unsubscribe', collection: c })
    this.subscribed = false
    this.subscribedCollections.clear()
    if (this.subscribeTimer) {
      clearTimeout(this.subscribeTimer)
      this.subscribeTimer = null
    }
  }

  async send(args: { type: 'broadcast'; event: string; payload: unknown }): Promise<{ status: string; error?: string }> {
    const ok = this.client.sendRaw({ type: 'broadcast', topic: this.topic, event: args.event, payload: args.payload })
    if (!ok) return { status: 'error', error: 'socket not connected' }
    return { status: 'ok' }
  }

  async track(state: Record<string, unknown>): Promise<{ status: string }> {
    const key = this.config?.presence?.key ?? 'anon'
    this.localPresence[key] = state
    this.client.sendRaw({ type: 'presence', topic: this.topic, event: 'track', key, state })
    // Local echo so presenceState() is useful before the Phase 16 backend lands.
    this.dispatchPresence({ event: 'join', key, current: state } as unknown as PresencePayload)
    return { status: 'ok' }
  }

  async untrack(): Promise<{ status: string }> {
    const key = this.config?.presence?.key ?? 'anon'
    delete this.localPresence[key]
    this.client.sendRaw({ type: 'presence', topic: this.topic, event: 'untrack', key })
    this.dispatchPresence({ event: 'leave', key } as unknown as PresencePayload)
    return { status: 'ok' }
  }

  presenceState(): Record<string, unknown> {
    return { ...this.localPresence }
  }

  // ---- inbound dispatch (called by RealtimeClient) ----

  dispatchChange(collection: string, event: string, data: Record<string, unknown>, mode: 'live' | 'polling', lagMs?: number): void {
    for (const b of this.bindings) {
      if (b.type !== 'postgres_changes') continue
      const f = b.filter as PostgresChangesFilter
      const table = f.table ?? f.collection
      if (table && table !== collection) continue
      const want = normalizeEvent(f.event ?? '*')
      if (want !== '*' && want !== event) continue
      if (!matchesRowFilter(f.filter, data)) continue
      ;(b.callback as (p: RealtimeChangePayload) => void)({ event, table: collection, data, mode, lagMs })
    }
  }

  dispatchBroadcast(event: string, payload: unknown): void {
    for (const b of this.bindings) {
      if (b.type !== 'broadcast') continue
      const f = b.filter as BroadcastFilter
      if (f.event && f.event !== '*' && f.event !== event) continue
      ;(b.callback as (p: BroadcastPayload) => void)({ event, payload })
    }
  }

  dispatchPresence(msg: PresencePayload): void {
    const rec = msg as unknown as Record<string, unknown>
    const event = String(rec['event'] ?? 'sync')
    if (event === 'join' && rec['key']) {
      this.localPresence[String(rec['key'])] = rec['current'] ?? true
    }
    if (event === 'leave' && rec['key']) {
      delete this.localPresence[String(rec['key'])]
    }
    for (const b of this.bindings) {
      if (b.type !== 'presence') continue
      const f = b.filter as PresenceFilter
      if (f.event && f.event !== '*' && f.event !== event) continue
      ;(b.callback as (p: PresencePayload) => void)({ ...msg, event })
    }
  }

  handleSubscribed(collection: string): void {
    this.subscribedCollections.add(collection)
    const needed = this.requiredCollections()
    if (needed.every((c) => this.subscribedCollections.has(c))) {
      this.subscribed = true
      if (this.subscribeTimer) {
        clearTimeout(this.subscribeTimer)
        this.subscribeTimer = null
      }
      this.subscribeCb?.('SUBSCRIBED')
    }
  }

  handleChannelError(collection: string | undefined, message: string): void {
    if (collection) {
      const needed = this.requiredCollections()
      if (!needed.includes(collection)) return
    }
    try {
      this.errorCb?.(message)
    } catch {
      // ignore
    }
    this.subscribeCb?.('CHANNEL_ERROR')
  }

  handleSocketClosed(): void {
    this.subscribed = false
    this.subscribeCb?.('CLOSED')
  }
}
