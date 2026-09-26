// AuthClient — GoTrue-compatible end-user auth for Openbase projects.
// Targets `<baseUrl>/auth/v1/*` (Track 1 / PHASES.md Phase 10). Every method
// follows the never-throw convention: `{ data, error }`.

import { Transport, type FetchImpl } from '../lib/fetch.js'
import { defaultStorage, type SessionStorageAdapter } from '../lib/storage.js'
import { singleflight } from '../lib/helpers.js'
import type { AuthChangeCallback, AuthChangeEvent, Session, User } from '../lib/types.js'

export type OtpType = 'signup' | 'recovery' | 'magiclink' | 'email_change' | 'sms' | 'phone_change'
export type OAuthProvider = 'github' | 'google' | 'apple' | 'azure' | 'discord' | 'oidc' | string
export type SignOutScope = 'global' | 'local' | 'others'

export interface AuthClientOptions {
  storage?: SessionStorageAdapter
  storageKey?: string
  persistSession?: boolean
  autoRefreshToken?: boolean
  detectSessionInUrl?: boolean
  flowType?: 'pkce' | 'implicit'
  fetch?: FetchImpl
  debug?: boolean
}

interface StoredPayload {
  session: Session
}

function nowSec(): number {
  return Math.floor(Date.now() / 1000)
}

function withExpiresAt(session: Session): Session {
  if (session.expires_at) return session
  const expires_in = session.expires_in ?? 3600
  return { ...session, expires_at: nowSec() + expires_in }
}

function sessionFromResponse(json: unknown): Session | null {
  if (!json || typeof json !== 'object') return null
  const j = json as Record<string, unknown>
  const session = (j['session'] ?? j) as Record<string, unknown>
  if (typeof session['access_token'] !== 'string' || typeof session['refresh_token'] !== 'string') return null
  const user = (session['user'] ?? j['user'] ?? {}) as User
  return withExpiresAt({
    access_token: session['access_token'] as string,
    refresh_token: session['refresh_token'] as string,
    token_type: (session['token_type'] as string) ?? 'bearer',
    expires_in: (session['expires_in'] as number) ?? 3600,
    expires_at: (session['expires_at'] as number) ?? nowSec() + ((session['expires_in'] as number) ?? 3600),
    user,
  })
}

export class AuthClient {
  private transport: Transport
  private storage: SessionStorageAdapter
  private storageKey: string
  private persistSession: boolean
  private autoRefreshToken: boolean
  private flowType: 'pkce' | 'implicit'
  private fetchImpl?: FetchImpl
  private listeners = new Map<string, AuthChangeCallback>()
  private listenerSeq = 0
  private refreshLock = singleflight<Session | null>()
  private refreshTimer: ReturnType<typeof setTimeout> | null = null
  private currentSession: Session | null = null
  private initialized = false

  constructor(baseUrl: string, apikey: string, opts: AuthClientOptions = {}) {
    this.transport = new Transport({ baseUrl, apikey, accessToken: () => this.currentSession?.access_token ?? null, fetch: opts.fetch })
    this.storage = opts.storage ?? defaultStorage()
    this.storageKey = opts.storageKey ?? 'ob-auth-session'
    this.persistSession = opts.persistSession ?? true
    this.autoRefreshToken = opts.autoRefreshToken ?? true
    this.flowType = opts.flowType ?? 'pkce'
    this.fetchImpl = opts.fetch
    if (opts.detectSessionInUrl ?? true) {
      void this.detectSessionInUrl().catch(() => undefined)
    }
  }

  // ---- internal plumbing ----

  private emit(event: AuthChangeEvent, session: Session | null): void {
    for (const cb of this.listeners.values()) {
      try {
        cb(event, session)
      } catch {
        // A throwing subscriber must never break auth state.
      }
    }
  }

  private scheduleRefresh(session: Session | null): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = null
    }
    if (!this.autoRefreshToken || !session?.expires_at || !session.refresh_token) return
    const delayMs = Math.max(0, (session.expires_at - nowSec() - 60) * 1000)
    // Cap at 12h so a far-future expiry cannot hold a timer forever.
    this.refreshTimer = setTimeout(() => {
      void this.refreshSession().then(({ data }) => {
        if (!data.session) this.emit('SIGNED_OUT', null)
      })
    }, Math.min(delayMs, 12 * 3600 * 1000))
    // Don't hold the process open for auth refresh in Node.
    const t = this.refreshTimer as unknown as { unref?: () => void }
    if (typeof t.unref === 'function') t.unref()
  }

  private async persist(session: Session | null, event: AuthChangeEvent | null): Promise<void> {
    this.currentSession = session
    this.transport.setAccessToken(() => this.currentSession?.access_token ?? null)
    this.scheduleRefresh(session)
    if (this.persistSession) {
      if (session) {
        await this.storage.setItem(this.storageKey, JSON.stringify({ session } satisfies StoredPayload))
      } else {
        await this.storage.removeItem(this.storageKey)
      }
    }
    if (event) this.emit(event, session)
  }

  /** Load persisted session (call once at startup; idempotent). */
  async initialize(): Promise<{ session: Session | null }> {
    if (this.initialized) return { session: this.currentSession }
    this.initialized = true
    try {
      const raw = await this.storage.getItem(this.storageKey)
      if (raw) {
        const parsed = JSON.parse(raw) as StoredPayload
        if (parsed?.session?.access_token) {
          const session = withExpiresAt(parsed.session)
          // Proactively refresh if expired or within the 60s window.
          if (session.expires_at && session.expires_at <= nowSec() + 60 && session.refresh_token && this.autoRefreshToken) {
            await this.refreshSession()
          } else {
            await this.persist(session, null)
          }
        }
      }
    } catch {
      // Corrupt storage must never break startup.
    }
    return { session: this.currentSession }
  }

  /** Parse implicit tokens or PKCE code from the browser URL (called automatically). */
  async detectSessionInUrl(): Promise<{ session: Session | null }> {
    if (typeof window === 'undefined' || !window.location) return { session: null }
    try {
      const url = new URL(window.location.href)
      const code = url.searchParams.get('code')
      if (code) {
        const verifier = window.sessionStorage?.getItem(`${this.storageKey}-pkce`) ?? undefined
        const { data } = await this.exchangeCodeForSession(code, verifier ?? undefined)
        // Clean the URL so the code is not left in history.
        url.searchParams.delete('code')
        window.history?.replaceState?.({}, '', url.toString())
        return { session: data.session }
      }
      const hash = new URLSearchParams(window.location.hash.replace(/^#/, ''))
      const access_token = hash.get('access_token')
      const refresh_token = hash.get('refresh_token')
      if (access_token && refresh_token) {
        const { data } = await this.setSession({ access_token, refresh_token })
        return { session: data.session }
      }
    } catch {
      // URL parsing must never throw.
    }
    return { session: null }
  }

  onAuthStateChange(cb: AuthChangeCallback): { data: { subscription: { unsubscribe: () => void } } } {
    const id = `sub_${++this.listenerSeq}`
    this.listeners.set(id, cb)
    return {
      data: {
        subscription: {
          unsubscribe: () => {
            this.listeners.delete(id)
          },
        },
      },
    }
  }

  // ---- session primitives ----

  async getSession(): Promise<{ data: { session: Session | null }; error: null }> {
    if (!this.initialized) await this.initialize()
    return { data: { session: this.currentSession }, error: null }
  }

  async setSession(tokens: {
    access_token: string
    refresh_token: string
  }): Promise<{ data: { session: Session | null; user: User | null }; error: null }> {
    const res = await this.transport.request<Session>('/auth/v1/token?grant_type=refresh_token', {
      method: 'POST',
      body: { refresh_token: tokens.refresh_token },
      fetch: this.fetchImpl,
    })
    // If the server echoes a session, prefer it; otherwise synthesize from tokens.
    const serverSession = res.data ? sessionFromResponse(res.data) : null
    const session =
      serverSession ??
      withExpiresAt({
        access_token: tokens.access_token,
        refresh_token: tokens.refresh_token,
        token_type: 'bearer',
        expires_in: 3600,
        user: this.currentSession?.user ?? ({ id: 'unknown' } as User),
      })
    await this.persist(session, 'SIGNED_IN')
    return { data: { session, user: session.user }, error: null }
  }

  /** Read storage tolerantly (sync or async adapters, never throws). */
  private async readStored(): Promise<string | null> {
    try {
      return await this.storage.getItem(this.storageKey)
    } catch {
      return null
    }
  }

  async refreshSession(): Promise<{ data: { session: Session | null; user: User | null }; error: null }> {
    const current = this.currentSession
    if (!current?.refresh_token) {
      const stored = await this.readStored()
      if (!stored) return { data: { session: null, user: null }, error: null }
    }
    const session = await this.refreshLock.run(async () => {
      const raw = await this.readStored()
      let storedRt: string | undefined
      try {
        storedRt = (JSON.parse(raw ?? '{}') as StoredPayload)?.session?.refresh_token
      } catch {
        storedRt = undefined
      }
      const rt = this.currentSession?.refresh_token ?? storedRt
      if (!rt) return null
      const res = await this.transport.request<Record<string, unknown>>('/auth/v1/token?grant_type=refresh_token', {
        method: 'POST',
        body: { refresh_token: rt },
        fetch: this.fetchImpl,
      })
      const next = res.data ? sessionFromResponse(res.data) : null
      if (!next) return null
      await this.persist(next, 'TOKEN_REFRESHED')
      return next
    })
    return { data: { session, user: session?.user ?? null }, error: null }
  }

  // ---- email / password / otp / oauth / anon ----

  async signUp(args: {
    email: string
    password: string
    options?: { data?: Record<string, unknown>; redirectTo?: string }
  }): Promise<{ data: { user: User | null; session: Session | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/signup', {
      method: 'POST',
      body: { email: args.email, password: args.password, data: args.options?.data ?? {}, redirect_to: args.options?.redirectTo },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, 'SIGNED_IN')
    return { data: { user, session }, error: null }
  }

  async signInWithPassword(args: {
    email: string
    password: string
  }): Promise<{ data: { user: User | null; session: Session | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/token?grant_type=password', {
      method: 'POST',
      body: { email: args.email, password: args.password },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, 'SIGNED_IN')
    return { data: { user, session }, error: null }
  }

  async signInWithOtp(
    args: ({ email: string } | { phone: string }) & { options?: { redirectTo?: string; data?: Record<string, unknown>; shouldCreateUser?: boolean } },
  ): Promise<{ data: { user: null; session: null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const body =
      'email' in args
        ? { email: args.email, create_user: args.options?.shouldCreateUser ?? true, data: args.options?.data ?? {}, redirect_to: args.options?.redirectTo }
        : { phone: args.phone, create_user: args.options?.shouldCreateUser ?? true, data: args.options?.data ?? {} }
    const res = await this.transport.request('/auth/v1/otp', { method: 'POST', body, fetch: this.fetchImpl })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    return { data: { user: null, session: null }, error: null }
  }

  async verifyOtp(args: {
    email?: string
    phone?: string
    token: string
    type: OtpType
  }): Promise<{ data: { user: User | null; session: Session | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/verify', {
      method: 'POST',
      body: { email: args.email, phone: args.phone, token: args.token, type: args.type },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, args.type === 'recovery' ? 'PASSWORD_RECOVERY' : 'SIGNED_IN')
    else if (args.type === 'recovery') this.emit('PASSWORD_RECOVERY', null)
    return { data: { user, session }, error: null }
  }

  async signInWithOAuth(args: {
    provider: OAuthProvider
    options?: { redirectTo?: string; scopes?: string; queryParams?: Record<string, string> }
  }): Promise<{ data: { url: string; provider: string }; error: null }> {
    const base = this.transport.baseUrl
    const params = new URLSearchParams({ provider: args.provider })
    // Browsers cannot send headers on navigation: the key travels as a query
    // parameter (accepted by requireAPIKey; never logged).
    params.set('apiKey', this.transport.apikey)
    if (args.options?.redirectTo) params.set('redirect_to', args.options.redirectTo)
    if (args.options?.scopes) params.set('scopes', args.options.scopes)
    if (this.flowType) params.set('flow_type', this.flowType)
    for (const [k, v] of Object.entries(args.options?.queryParams ?? {})) params.set(k, v)
    // PKCE verifier is stored so detectSessionInUrl can exchange the code.
    if (typeof window !== 'undefined' && window.sessionStorage) {
      try {
        const verifier = [...crypto.getRandomValues(new Uint8Array(32))].map((b) => b.toString(16).padStart(2, '0')).join('')
        window.sessionStorage.setItem(`${this.storageKey}-pkce`, verifier)
        params.set('code_challenge', verifier)
        params.set('code_challenge_method', 'plain')
      } catch {
        // PKCE is best-effort in non-secure contexts.
      }
    }
    return { data: { url: `${base}/auth/v1/authorize?${params.toString()}`, provider: args.provider }, error: null }
  }

  async signInAnonymously(): Promise<{ data: { user: User | null; session: Session | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/otp', {
      method: 'POST',
      body: { anonymous: true },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, 'SIGNED_IN')
    return { data: { user, session }, error: null }
  }

  async signInWithIdToken(args: {
    provider: 'google' | 'apple' | 'azure' | 'facebook' | string
    token: string
    nonce?: string
  }): Promise<{ data: { user: User | null; session: Session | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/token?grant_type=id_token', {
      method: 'POST',
      body: { provider: args.provider, id_token: args.token, nonce: args.nonce },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { user: null, session: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, 'SIGNED_IN')
    return { data: { user, session }, error: null }
  }

  async linkIdentity(args: {
    provider: OAuthProvider
    options?: { redirectTo?: string; scopes?: string }
  }): Promise<{ data: { url: string }; error: null }> {
    // Anonymous upgrade: same authorize flow with an identity-link hint; the
    // server links instead of creating when the caller has a session.
    const oauth = await this.signInWithOAuth(args)
    return { data: { url: oauth.data.url }, error: null }
  }

  async exchangeCodeForSession(
    code: string,
    codeVerifier?: string,
  ): Promise<{ data: { session: Session | null; user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/token?grant_type=pkce', {
      method: 'POST',
      body: { auth_code: code, code_verifier: codeVerifier },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: { session: null, user: null }, error: res.error }
    const session = sessionFromResponse(res.data)
    const user = ((res.data as Record<string, unknown>)?.['user'] as User) ?? session?.user ?? null
    if (session) await this.persist(session, 'SIGNED_IN')
    return { data: { session, user }, error: null }
  }

  async getUser(): Promise<{ data: { user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<User>('/auth/v1/user', { method: 'GET', fetch: this.fetchImpl })
    if (res.error) return { data: { user: null }, error: res.error }
    return { data: { user: res.data }, error: null }
  }

  async updateUser(attrs: {
    email?: string
    password?: string
    phone?: string
    data?: Record<string, unknown>
  }): Promise<{ data: { user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request<Record<string, unknown>>('/auth/v1/user', { method: 'PUT', body: attrs, fetch: this.fetchImpl })
    if (res.error) return { data: { user: null }, error: res.error }
    // The server returns {user} plus, when it rotated the session (e.g.
    // password change), a fresh {session} to adopt.
    const body = (res.data ?? {}) as { user?: User; session?: Record<string, unknown> }
    const user = body.user ?? (res.data as User)
    const rotated = body.session ? sessionFromResponse(body.session) : null
    if (rotated) {
      await this.persist(rotated, 'USER_UPDATED')
      return { data: { user: rotated.user ?? user ?? null }, error: null }
    }
    if (this.currentSession) {
      const next: Session = { ...this.currentSession, user: user ?? this.currentSession.user }
      await this.persist(next, 'USER_UPDATED')
    } else {
      this.emit('USER_UPDATED', null)
    }
    return { data: { user: user ?? null }, error: null }
  }

  async resetPasswordForEmail(
    email: string,
    options?: { redirectTo?: string },
  ): Promise<{ data: Record<string, never>; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request('/auth/v1/recover', {
      method: 'POST',
      body: { email, redirect_to: options?.redirectTo },
      fetch: this.fetchImpl,
    })
    if (res.error) return { data: {}, error: res.error }
    return { data: {}, error: null }
  }

  async resend(args: {
    type: 'signup' | 'email_change' | 'sms' | 'phone_change'
    email?: string
    phone?: string
  }): Promise<{ data: Record<string, never>; error: import('../lib/fetch.js').Err['error'] | null }> {
    const res = await this.transport.request('/auth/v1/resend', { method: 'POST', body: args, fetch: this.fetchImpl })
    if (res.error) return { data: {}, error: res.error }
    return { data: {}, error: null }
  }

  async signOut(scope: SignOutScope = 'global'): Promise<{ error: import('../lib/fetch.js').Err['error'] | null }> {
    if (scope === 'global' || scope === 'others') {
      await this.transport.request('/auth/v1/logout', { method: 'POST', body: { scope }, fetch: this.fetchImpl })
    }
    if (scope === 'global' || scope === 'local') {
      await this.persist(null, 'SIGNED_OUT')
    }
    return { error: null }
  }

  // ---- MFA (TOTP) ----

  readonly mfa = {
    enroll: async (args: { factorType: 'totp'; friendlyName?: string }): Promise<{
      data: { id: string; totp?: { qr_code: string; secret: string; uri: string } } | null
      error: import('../lib/fetch.js').Err['error'] | null
    }> => {
      const res = await this.transport.request<{ id: string; totp?: { qr_code: string; secret: string; uri: string } }>(
        '/auth/v1/factors',
        { method: 'POST', body: { factor_type: args.factorType, friendly_name: args.friendlyName }, fetch: this.fetchImpl },
      )
      if (res.error) return { data: null, error: res.error }
      return { data: res.data, error: null }
    },
    challenge: async (args: { factorId: string }): Promise<{
      data: { id: string } | null
      error: import('../lib/fetch.js').Err['error'] | null
    }> => {
      const res = await this.transport.request<{ id: string }>(`/auth/v1/factors/${args.factorId}/challenge`, {
        method: 'POST',
        body: {},
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: null, error: res.error }
      return { data: res.data, error: null }
    },
    verify: async (args: { factorId: string; challengeId: string; code: string }): Promise<{
      data: { access_token: string; token_type: string } | null
      error: import('../lib/fetch.js').Err['error'] | null
    }> => {
      const res = await this.transport.request<{ access_token: string; token_type: string }>(
        `/auth/v1/factors/${args.factorId}/verify`,
        { method: 'POST', body: { challenge_id: args.challengeId, code: args.code }, fetch: this.fetchImpl },
      )
      if (res.error) return { data: null, error: res.error }
      this.emit('MFA_CHALLENGE_VERIFIED', this.currentSession)
      return { data: res.data, error: null }
    },
    unenroll: async (args: { factorId: string }): Promise<{ data: Record<string, never>; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request(`/auth/v1/factors/${args.factorId}`, {
        method: 'DELETE',
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: {}, error: res.error }
      return { data: {}, error: null }
    },
    listFactors: async (): Promise<{
      data: { totp: Array<{ id: string; friendly_name?: string; status: string }> } | null
      error: import('../lib/fetch.js').Err['error'] | null
    }> => {
      const res = await this.transport.request<{ totp: Array<{ id: string; friendly_name?: string; status: string }> }>(
        '/auth/v1/factors',
        { method: 'GET', fetch: this.fetchImpl },
      )
      if (res.error) return { data: null, error: res.error }
      return { data: res.data, error: null }
    },
  }

  // ---- admin (service_role only; server-side) ----

  readonly admin = {
    listUsers: async (): Promise<{ data: { users: User[] } | null; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request<{ users: User[] }>('/auth/v1/admin/users', {
        method: 'GET',
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: null, error: res.error }
      return { data: res.data, error: null }
    },
    createUser: async (attrs: {
      email: string
      password?: string
      email_confirm?: boolean
      user_metadata?: Record<string, unknown>
    }): Promise<{ data: { user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request<{ user?: User } & User>('/auth/v1/admin/users', {
        method: 'POST',
        body: attrs,
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: { user: null }, error: res.error }
      return { data: { user: res.data?.user ?? (res.data as User) ?? null }, error: null }
    },
    deleteUser: async (id: string): Promise<{ data: Record<string, never>; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request(`/auth/v1/admin/users/${id}`, { method: 'DELETE', fetch: this.fetchImpl })
      if (res.error) return { data: {}, error: res.error }
      return { data: {}, error: null }
    },
    inviteUserByEmail: async (
      email: string,
      options?: { redirectTo?: string; data?: Record<string, unknown> },
    ): Promise<{ data: { user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request<{ user?: User } & User>('/auth/v1/admin/invite', {
        method: 'POST',
        body: { email, redirect_to: options?.redirectTo, data: options?.data ?? {} },
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: { user: null }, error: res.error }
      return { data: { user: res.data?.user ?? (res.data as User) ?? null }, error: null }
    },
    generateLink: async (args: {
      type: 'signup' | 'magiclink' | 'recovery' | 'invite' | 'email_change_current' | 'email_change_new'
      email: string
      password?: string
    }): Promise<{ data: { action_link: string } | null; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request<{ action_link: string }>('/auth/v1/admin/generate_link', {
        method: 'POST',
        body: args,
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: null, error: res.error }
      return { data: res.data, error: null }
    },
    updateUserById: async (
      id: string,
      attrs: { email?: string; password?: string; ban_duration?: string; user_metadata?: Record<string, unknown> },
    ): Promise<{ data: { user: User | null }; error: import('../lib/fetch.js').Err['error'] | null }> => {
      const res = await this.transport.request<{ user?: User } & User>(`/auth/v1/admin/users/${id}`, {
        method: 'PUT',
        body: attrs,
        fetch: this.fetchImpl,
      })
      if (res.error) return { data: { user: null }, error: res.error }
      return { data: { user: res.data?.user ?? (res.data as User) ?? null }, error: null }
    },
  }

  /** Test hook: stop the refresh timer. */
  stopAutoRefresh(): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = null
    }
  }
}
