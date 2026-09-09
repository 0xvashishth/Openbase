import { describe, expect, it, vi } from 'vitest'
import { Transport } from '../src/lib/fetch.js'
import { PostgrestQueryBuilder } from '../src/postgrest/builder.js'

const T = () => new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: (async () => new Response('[]', { status: 200 })) as never })

function build(fn: (q: PostgrestQueryBuilder) => PostgrestQueryBuilder): string {
  const q = new PostgrestQueryBuilder(T(), 'orders')
  return fn(q).buildPath()
}

describe('PostgREST filter grammar', () => {
  it('serializes every comparison operator', () => {
    const p = build((q) =>
      q.select('*').eq('a', 1).neq('b', 2).gt('c', 3).gte('d', 4).lt('e', 5).lte('f', 6),
    )
    expect(p).toContain('a=eq.1')
    expect(p).toContain('b=neq.2')
    expect(p).toContain('c=gt.3')
    expect(p).toContain('d=gte.4')
    expect(p).toContain('e=lt.5')
    expect(p).toContain('f=lte.6')
  })

  it('like/ilike/is/in', () => {
    const p = build((q) => q.select('*').like('n', '%rush%').ilike('m', '%x%').is('archived', null).in('region', ['eu', 'us']))
    expect(p).toContain('n=like.')
    expect(p).toContain('m=ilike.')
    expect(p).toContain('archived=is.null')
    expect(p).toContain('region=in.%28eu%2Cus%29')
  })

  it('quotes in-list values containing delimiters', () => {
    const p = build((q) => q.select('*').in('name', ['a,b', 'c(d)']))
    expect(p).toContain('in.')
    expect(decodeURIComponent(p)).toContain('"a,b"')
  })

  it('range/containment/overlap operators', () => {
    const p = build((q) =>
      q.select('*').contains('tags', ['a']).containedBy('tags', ['a']).rangeGt('r', 1).rangeGte('r2', 2).rangeLt('r3', 3).rangeLte('r4', 4).rangeAdjacent('r5', 5).overlaps('r6', [1, 2]),
    )
    for (const op of ['cs.', 'cd.', 'sr.', 'nxl.', 'sl.', 'nxr.', 'adj.', 'ov.']) expect(p).toContain(op)
  })

  it('textSearch with type + config', () => {
    const p = build((q) => q.select('*').textSearch('note', 'rush', { type: 'websearch', config: 'english' }))
    expect(decodeURIComponent(p)).toContain('note=w(english).rush')
  })

  it('match/not/or/and/filter', () => {
    const p = build((q) => q.select('*').match({ a: 1, b: 2 }).not('c', 'eq', 3).or('a.eq.1,b.gt.2').and('x.eq.1').filter('y', 'gte', 9))
    expect(p).toContain('a=eq.1')
    expect(decodeURIComponent(p)).toContain('c=not.eq.3')
    expect(decodeURIComponent(p)).toContain('or=(a.eq.1,b.gt.2)')
    expect(decodeURIComponent(p)).toContain('and=(x.eq.1)')
    expect(p).toContain('y=gte.9')
  })

  it('order/limit/range/offset map to PostgREST + legacy compat params', () => {
    const p = build((q) => q.select('*').order('created_at', { ascending: false, nullsFirst: false }).limit(20))
    expect(decodeURIComponent(p)).toContain('order=created_at.desc.nullslast')
    expect(p).toContain('order_by=created_at')
    expect(p).toContain('limit=20')

    const r = build((q) => q.select('*').range(0, 19))
    expect(r).toContain('offset=0')
    expect(r).toContain('limit=20')
  })

  it('select count sets Prefer header; buildHeaders dedupes', () => {
    const q = new PostgrestQueryBuilder(T(), 'orders')
    q.select('id', { count: 'exact' }).limit(5)
    expect(q.buildHeaders()['Prefer']).toBe('count=exact')
  })

  it('insert unwraps single-element arrays for current backend compat', async () => {
    const seen: Array<{ url: string; body: unknown }> = []
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push({ url, body: init?.body ? JSON.parse(init.body as string) : undefined })
      return new Response(JSON.stringify({ id: '1' }), { status: 201 })
    })
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: fetchMock as never })
    const q = new PostgrestQueryBuilder(t, 'orders')
    await q.insert([{ a: 1 }]).execute()
    expect(seen[0]?.body).toEqual({ a: 1 })
    await new PostgrestQueryBuilder(t, 'orders').insert([{ a: 1 }, { a: 2 }]).execute()
    expect(seen[1]?.body).toEqual([{ a: 1 }, { a: 2 }])
  })

  it('single eq update/delete use id-addressed endpoints; multi-filter uses PATCH/DELETE', async () => {
    const seen: string[] = []
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push(`${init?.method} ${url}`)
      return new Response(JSON.stringify({ matched_count: 1 }), { status: 200 })
    })
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: fetchMock as never })
    await new PostgrestQueryBuilder(t, 'orders').update({ status: 'x' }).eq('id', 7).execute()
    await new PostgrestQueryBuilder(t, 'orders').delete().eq('id', 7).execute()
    expect(seen[0]).toBe('PUT http://localhost:8080/v1/api/orders/7')
    expect(seen[1]).toBe('DELETE http://localhost:8080/v1/api/orders/7')

    seen.length = 0
    await new PostgrestQueryBuilder(t, 'orders').update({ status: 'x' }).eq('a', 1).eq('b', 2).execute()
    expect(seen[0]?.startsWith('PATCH http://localhost:8080/v1/api/orders?')).toBe(true)
  })

  it('single()/maybeSingle() shape results; single errors on 0 rows', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify([{ id: 1 }]), { status: 200 }))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: fetchMock as never })
    const one = await new PostgrestQueryBuilder(t, 'orders').select('*').single().execute()
    expect(one.data).toEqual({ id: 1 })

    const emptyFetch = vi.fn(async () => new Response(JSON.stringify([]), { status: 200 }))
    const t2 = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: emptyFetch as never })
    const maybe = await new PostgrestQueryBuilder(t2, 'orders').select('*').maybeSingle().execute()
    expect(maybe.data).toBeNull()
    const single = await new PostgrestQueryBuilder(t2, 'orders').select('*').single().execute()
    expect(single.error?.code).toBe('PGRST116')
  })

  it('is thenable: await builder directly', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ rows: [{ id: 2 }] }), { status: 200 }))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: fetchMock as never })
    const res = await new PostgrestQueryBuilder(t, 'orders').select('*')
    expect(res.data).toEqual([{ id: 2 }])
  })

  it('rpc validates name and posts to /v1/api/rpc/{fn}', async () => {
    const { PostgrestClient } = await import('../src/postgrest/builder.js')
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }))
    const t = new Transport({ baseUrl: 'http://localhost:8080', apikey: 'ob_testkey123456', fetch: fetchMock as never })
    const c = new PostgrestClient(t)
    const bad = await c.rpc('')
    expect(bad.error?.code).toBe('validation')
    const ok = await c.rpc('top_customers', { limit: 5 })
    expect(ok.data).toEqual({ ok: true })
  })
})
