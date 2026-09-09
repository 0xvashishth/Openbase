import { describe, expect, it, vi } from 'vitest'
import { AuthClient } from '../src/auth/client.js'
import { MemoryStorage } from '../src/lib/storage.js'

const BASE = 'http://localhost:8080'
const KEY = 'ob_testkey123456'

function sessionJson(overrides: Record<string, unknown> = {}) {
  return {
    access_token: 'access-1',
    refresh_token: 'refresh-1',
    token_type: 'bearer',
    expires_in: 3600,
    user: { id: 'u1', email: 'a@x.com' },
    ...overrides,
  }
}

function mockFetchOnce(body: unknown, status = 200) {
  return vi.fn(async () =>
    new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } }),
  )
}

describe('AuthClient sessions', () => {
  it('signInWithPassword persists session and emits SIGNED_IN', async () => {
    const storage = new MemoryStorage()
    const fetchMock = mockFetchOnce(sessionJson())
    const auth = new AuthClient(BASE, KEY, { storage, fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    const events: string[] = []
    auth.onAuthStateChange((e) => events.push(e))
    const { data, error } = await auth.signInWithPassword({ email: 'a@x.com', password: 'pw123456' })
    expect(error).toBeNull()
    expect(data.session?.access_token).toBe('access-1')
    expect(events).toEqual(['SIGNED_IN'])
    expect(await storage.getItem('ob-auth-session')).toContain('access-1')
    const { data: got } = await auth.getSession()
    expect(got.session?.user.id).toBe('u1')
  })

  it('signUp returns error shape without throwing', async () => {
    const fetchMock = mockFetchOnce({ error: 'email taken' }, 409)
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    const { data, error } = await auth.signUp({ email: 'a@x.com', password: 'pw123456' })
    expect(data.session).toBeNull()
    expect(error?.message).toBe('email taken')
  })

  it('verifyOtp with recovery emits PASSWORD_RECOVERY', async () => {
    const fetchMock = mockFetchOnce(sessionJson())
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    const events: string[] = []
    auth.onAuthStateChange((e) => events.push(e))
    await auth.verifyOtp({ email: 'a@x.com', token: '123456', type: 'recovery' })
    expect(events[0]).toBe('PASSWORD_RECOVERY')
  })

  it('signOut local clears storage and emits SIGNED_OUT', async () => {
    const storage = new MemoryStorage()
    const fetchMock = mockFetchOnce(sessionJson())
    const auth = new AuthClient(BASE, KEY, { storage, fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    await auth.signInWithPassword({ email: 'a@x.com', password: 'pw' })
    const events: string[] = []
    auth.onAuthStateChange((e) => events.push(e))
    await auth.signOut('local')
    expect(events).toEqual(['SIGNED_OUT'])
    expect(await storage.getItem('ob-auth-session')).toBeNull()
  })

  it('initialize restores persisted session; corrupt storage is ignored', async () => {
    const storage = new MemoryStorage()
    await storage.setItem('ob-auth-session', JSON.stringify({ session: sessionJson() }))
    const auth = new AuthClient(BASE, KEY, { storage, fetch: mockFetchOnce({}) as never, detectSessionInUrl: false, autoRefreshToken: false })
    const { session } = await auth.initialize()
    expect(session?.access_token).toBe('access-1')

    const bad = new MemoryStorage()
    await bad.setItem('ob-auth-session', 'not-json{{{')
    const auth2 = new AuthClient(BASE, KEY, { storage: bad, fetch: mockFetchOnce({}) as never, detectSessionInUrl: false, autoRefreshToken: false })
    const r2 = await auth2.initialize()
    expect(r2.session).toBeNull()
  })

  it('refreshSession single-flights concurrent callers', async () => {
    let calls = 0
    const fetchMock = vi.fn(async () => {
      calls++
      await new Promise((r) => setTimeout(r, 20))
      return new Response(JSON.stringify(sessionJson({ access_token: `a${calls}` })), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      })
    })
    const storage = new MemoryStorage()
    await storage.setItem('ob-auth-session', JSON.stringify({ session: sessionJson() }))
    const auth = new AuthClient(BASE, KEY, { storage, fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    await auth.initialize()
    const [r1, r2] = await Promise.all([auth.refreshSession(), auth.refreshSession()])
    expect(calls).toBe(1)
    expect(r1.data.session?.access_token).toBe(r2.data.session?.access_token)
  })

  it('signInWithOAuth builds authorize URL with provider + redirect', async () => {
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: mockFetchOnce({}) as never, detectSessionInUrl: false, autoRefreshToken: false })
    const { data } = await auth.signInWithOAuth({ provider: 'github', options: { redirectTo: 'https://app.com/cb', scopes: 'read:user' } })
    expect(data.url).toContain('/auth/v1/authorize?')
    expect(data.url).toContain('provider=github')
    expect(data.url).toContain('redirect_to=')
  })

  it('mfa enroll/challenge/verify/list/unenroll hit factor endpoints', async () => {
    const seen: string[] = []
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push(`${init?.method} ${url}`)
      if (url.includes('/factors') && init?.method === 'POST' && !url.includes('challenge') && !url.includes('verify')) {
        return new Response(JSON.stringify({ id: 'f1', totp: { qr_code: 'qr', secret: 's', uri: 'u' } }), { status: 200 })
      }
      if (url.includes('challenge')) return new Response(JSON.stringify({ id: 'c1' }), { status: 200 })
      if (url.includes('verify')) return new Response(JSON.stringify({ access_token: 'a', token_type: 'bearer' }), { status: 200 })
      if (init?.method === 'GET') return new Response(JSON.stringify({ totp: [] }), { status: 200 })
      return new Response(JSON.stringify({}), { status: 200 })
    })
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    await auth.mfa.enroll({ factorType: 'totp' })
    await auth.mfa.challenge({ factorId: 'f1' })
    await auth.mfa.verify({ factorId: 'f1', challengeId: 'c1', code: '123456' })
    await auth.mfa.listFactors()
    await auth.mfa.unenroll({ factorId: 'f1' })
    expect(seen.join('\n')).toContain('/auth/v1/factors')
    expect(seen.join('\n')).toContain('challenge')
    expect(seen.join('\n')).toContain('verify')
  })

  it('admin methods call /auth/v1/admin/*', async () => {
    const seen: string[] = []
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push(`${init?.method} ${url}`)
      return new Response(JSON.stringify({ users: [], user: { id: 'u9' }, action_link: 'http://x' }), { status: 200 })
    })
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    await auth.admin.listUsers()
    await auth.admin.createUser({ email: 'b@x.com' })
    await auth.admin.deleteUser('u9')
    await auth.admin.inviteUserByEmail('c@x.com')
    await auth.admin.generateLink({ type: 'signup', email: 'd@x.com' })
    expect(seen.every((s) => s.includes('/auth/v1/admin/'))).toBe(true)
    expect(seen).toHaveLength(5)
  })

  it('onAuthStateChange unsubscribe stops events', async () => {
    const fetchMock = mockFetchOnce(sessionJson())
    const auth = new AuthClient(BASE, KEY, { storage: new MemoryStorage(), fetch: fetchMock as never, detectSessionInUrl: false, autoRefreshToken: false })
    let n = 0
    const { data } = auth.onAuthStateChange(() => n++)
    data.subscription.unsubscribe()
    await auth.signInWithPassword({ email: 'a@x.com', password: 'pw' })
    expect(n).toBe(0)
  })
})
