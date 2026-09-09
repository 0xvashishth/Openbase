import { describe, expect, it, vi } from 'vitest'
import { Transport, parseCountFromContentRange } from '../src/lib/fetch.js'

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', ...headers },
  })
}

describe('parseCountFromContentRange', () => {
  it('parses exact counts and nulls', () => {
    expect(parseCountFromContentRange('0-19/42')).toBe(42)
    expect(parseCountFromContentRange('0-19/*')).toBeNull()
    expect(parseCountFromContentRange(null)).toBeNull()
    expect(parseCountFromContentRange('garbage')).toBeNull()
  })
})

describe('Transport', () => {
  it('attaches apikey + Bearer and parses JSON', async () => {
    const fetchMock = vi.fn(async () => jsonResponse([{ id: 1 }]))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123', fetch: fetchMock as never })
    const res = await t.request<Array<{ id: number }>>('/v1/api/tables')
    expect(res.error).toBeNull()
    expect(res.data).toEqual([{ id: 1 }])
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    const h = init.headers as Record<string, string>
    expect(h['apikey']).toBe('ob_testkey123')
    expect(h['Authorization']).toBe('Bearer ob_testkey123')
  })

  it('prefers the end-user access token when set', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true }))
    const t = new Transport({
      baseUrl: 'http://localhost:8080',
      apikey: 'ob_testkey123',
      accessToken: () => 'jwt-user-token',
      fetch: fetchMock as never,
    })
    await t.request('/auth/v1/user')
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect((init.headers as Record<string, string>)['Authorization']).toBe('Bearer jwt-user-token')
  })

  it('returns typed error (never throws) on HTTP failure', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ error: 'invalid API key' }, 401))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123', fetch: fetchMock as never })
    const res = await t.request('/v1/api/tables')
    expect(res.data).toBeNull()
    expect(res.error?.message).toBe('invalid API key')
    expect(res.status).toBe(401)
  })

  it('returns network error shape when fetch rejects', async () => {
    const fetchMock = vi.fn(async () => {
      throw new Error('boom')
    })
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123', fetch: fetchMock as never })
    const res = await t.request('/v1/api/tables')
    expect(res.data).toBeNull()
    expect(res.error?.code).toBe('network')
  })

  it('retries GET once on 503 then succeeds', async () => {
    const fetchMock = vi
      .fn(async () => jsonResponse({ error: 'x' }, 503))
      .mockImplementationOnce(async () => jsonResponse({ error: 'x' }, 503))
      .mockImplementationOnce(async () => jsonResponse([{ a: 1 }], 200))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123', fetch: fetchMock as never })
    const res = await t.request('/v1/api/tables')
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(res.error).toBeNull()
  })

  it('surfaces Content-Range count', async () => {
    const fetchMock = vi.fn(async () => jsonResponse([], 206, { 'content-range': '0-19/42' }))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123', fetch: fetchMock as never })
    const res = await t.request('/v1/api/tables')
    expect(res.count).toBe(42)
  })
})
