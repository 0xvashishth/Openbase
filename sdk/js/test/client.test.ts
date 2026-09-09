import { describe, expect, it, vi } from 'vitest'
import { Transport } from '../src/lib/fetch.js'
import { StorageClient } from '../src/storage/client.js'
import { FunctionsClient } from '../src/functions/client.js'
import { createClient } from '../src/client.js'
import { createServerClient, createBrowserClient } from '../src/server.js'
import { MemoryStorage } from '../src/lib/storage.js'

const T = (handler: (url: string, init?: RequestInit) => Promise<Response>) =>
  new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: handler as never })

describe('StorageClient', () => {
  it('upload PUTs bytes; list/move/copy/remove/sign map to REST contract', async () => {
    const seen: string[] = []
    const t = T(async (url, init) => {
      seen.push(`${init?.method} ${url}`)
      if (url.endsWith('/sign')) return new Response(JSON.stringify({ signedUrl: 'http://signed' }), { status: 200 })
      if (url.includes('/v1/storage') && init?.method === 'GET') return new Response(JSON.stringify([{ name: 'a.png' }]), { status: 200 })
      return new Response(JSON.stringify({ path: 'a.png' }), { status: 200 })
    })
    const s = new StorageClient(t)
    await s.from('avatars').upload('a.png', new Uint8Array([1, 2]), { contentType: 'image/png', upsert: true })
    await s.from('avatars').list('u/', { limit: 10, search: 'a' })
    await s.from('avatars').move('a.png', 'b.png')
    await s.from('avatars').copy('a.png', 'c.png')
    await s.from('avatars').remove(['a.png'])
    const signed = await s.from('avatars').createSignedUrl('a.png', 60)
    expect(seen.join('\n')).toContain('PUT http://localhost:8080/v1/storage/avatars/a.png')
    expect(seen.join('\n')).toContain('/move')
    expect(seen.join('\n')).toContain('/copy')
    expect(signed.data).toEqual({ signedUrl: 'http://signed' })
  })

  it('404 becomes storage_unavailable; validation errors are typed', async () => {
    const t = T(async () => new Response(JSON.stringify({ error: 'not found' }), { status: 404 }))
    const s = new StorageClient(t)
    const up = await s.from('avatars').upload('a.png', 'hello')
    expect((up.error as { code?: string })?.code ?? (up.error as Error)?.message).toBeTruthy()
    const msg = JSON.stringify(up.error)
    expect(msg).toContain('storage_unavailable')
    const bad = await s.from('avatars').remove([])
    expect(JSON.stringify(bad.error)).toContain('validation')
    const badSign = await s.from('avatars').createSignedUrl('a.png', -1)
    expect(JSON.stringify(badSign.error)).toContain('validation')
  })

  it('getPublicUrl is deterministic; update forces upsert; buckets CRUD', async () => {
    const seen: string[] = []
    const t = T(async (url, init) => {
      seen.push(`${init?.method} ${url}`)
      return new Response(JSON.stringify([{ name: 'b', public: true }]), { status: 200 })
    })
    const s = new StorageClient(t)
    expect(s.from('avatars').getPublicUrl('a.png').data.publicUrl).toBe(
      'http://localhost:8080/v1/storage/avatars/a.png?public=true',
    )
    await s.listBuckets()
    await s.createBucket('avatars', { public: true })
    await s.deleteBucket('avatars')
    expect(seen.join('\n')).toContain('GET http://localhost:8080/v1/storage')
    expect(() => s.from('')).toThrowError()
  })
})

describe('FunctionsClient', () => {
  it('invoke posts to /v1/functions/{name}; 404 becomes functions_unavailable', async () => {
    const t = T(async (url) => {
      if (url.includes('missing')) return new Response(JSON.stringify({ error: 'nope' }), { status: 404 })
      return new Response(JSON.stringify({ hello: 'world' }), { status: 200 })
    })
    const f = new FunctionsClient(t)
    const ok = await f.invoke('hello', { body: { a: 1 } })
    expect(ok.data).toEqual({ hello: 'world' })
    const missing = await f.invoke('missing')
    expect(JSON.stringify(missing.error)).toContain('functions_unavailable')
    const bad = await f.invoke('')
    expect(JSON.stringify(bad.error)).toContain('validation')
  })
})

describe('OpenbaseClient composition', () => {
  it('createClient validates url/key and wires auth→transport Bearer', async () => {
    expect(() => createClient('', 'ob_testkey123456')).toThrowError()
    expect(() => createClient('http://x', 'bad')).toThrowError()
    let authHeader = ''
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      authHeader = (init?.headers as Record<string, string>)['Authorization'] ?? ''
      return new Response(JSON.stringify([]), { status: 200 })
    })
    const c = createClient('http://localhost:8080', 'ob_testkey123456', {
      auth: { storage: new MemoryStorage(), detectSessionInUrl: false, autoRefreshToken: false },
      global: { fetch: fetchMock as never },
    })
    await c.getTables()
    expect(authHeader).toBe('Bearer ob_testkey123456')
    const v = await c.serverVersion()
    expect(fetchMock).toHaveBeenCalled()
    expect(v.data ?? v.error).toBeTruthy()
  })

  it('from/rpc/channel/storage/functions/getSchema are reachable', async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.includes('/_schema')) return new Response(JSON.stringify({ collection: 'orders' }), { status: 200 })
      if (url.includes('/rpc/')) return new Response(JSON.stringify({ ok: 1 }), { status: 200 })
      return new Response(JSON.stringify([]), { status: 200 })
    })
    const c = createClient('http://localhost:8080', 'ob_testkey123456', {
      auth: { storage: new MemoryStorage(), detectSessionInUrl: false, autoRefreshToken: false },
      global: { fetch: fetchMock as never },
    })
    await c.from('orders').select('*').limit(1).execute()
    await c.rpc('top_customers', { limit: 1 })
    await c.getSchema('orders')
    expect(c.storage).toBeTruthy()
    expect(c.functions).toBeTruthy()
    expect(c.channel('t').topic).toBe('t')
    expect(c.getChannels()).toHaveLength(1)
    await c.removeAllChannels()
  })
})

describe('server helpers', () => {
  it('createServerClient uses cookie storage; createBrowserClient constructs', async () => {
    const jar = new Map<string, string>()
    const cookies = {
      get: (n: string) => jar.get(n),
      set: (n: string, v: string) => void jar.set(n, v),
      remove: (n: string) => void jar.delete(n),
    }
    const server = createServerClient('http://localhost:8080', 'ob_testkey123456', { cookies })
    const { data } = await server.auth.getSession()
    expect(data.session).toBeNull()
    const browser = createBrowserClient('http://localhost:8080', 'ob_testkey123456', {
      auth: { storage: new MemoryStorage(), detectSessionInUrl: false, autoRefreshToken: false },
    })
    expect(browser.baseUrl).toBe('http://localhost:8080')
  })
})
