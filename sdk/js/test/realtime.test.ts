import { describe, expect, it, vi } from 'vitest'
import { RealtimeClient, matchesRowFilter, type WebSocketLike } from '../src/realtime/channel.js'

function fakeSocket(): WebSocketLike & { sent: string[]; serverMessage: (raw: string) => void } {
  const s = {
    readyState: 1,
    OPEN: 1,
    sent: [] as string[],
    onopen: null as WebSocketLike['onopen'],
    onmessage: null as WebSocketLike['onmessage'],
    onclose: null as WebSocketLike['onclose'],
    onerror: null as WebSocketLike['onerror'],
    send(data: string) {
      s.sent.push(data)
    },
    close() {},
    serverMessage(raw: string) {
      s.onmessage?.({ data: raw } as { data: string })
    },
  } satisfies WebSocketLike & { sent: string[]; serverMessage: (raw: string) => void }
  return s as WebSocketLike & { sent: string[]; serverMessage: (raw: string) => void }
}

function clientWithFake() {
  const sock = fakeSocket()
  const c = new RealtimeClient('ws://localhost:8080/v1/realtime', {
    apikey: 'ob_testkey123456',
    wsFactory: () => {
      queueMicrotask(() => sock.onopen?.({}))
      return sock
    },
    reconnectMs: 10,
  })
  return { client: c, sock }
}

describe('matchesRowFilter', () => {
  it('eq/neq/gt/gte/lt/lte + unknown passthrough', () => {
    expect(matchesRowFilter('status=eq.open', { status: 'open' })).toBe(true)
    expect(matchesRowFilter('status=eq.open', { status: 'closed' })).toBe(false)
    expect(matchesRowFilter('status=neq.open', { status: 'closed' })).toBe(true)
    expect(matchesRowFilter('n=gt.3', { n: 5 })).toBe(true)
    expect(matchesRowFilter('n=lte.3', { n: 4 })).toBe(false)
    expect(matchesRowFilter(undefined, {})).toBe(true)
    expect(matchesRowFilter('weird', {})).toBe(true)
  })
})

describe('RealtimeClient channels', () => {
  it('subscribe sends gateway subscribe per table; subscribed ack fires SUBSCRIBED', async () => {
    const { client, sock } = clientWithFake()
    const ch = client.channel('orders-feed')
    const statuses: string[] = []
    ch.on('postgres_changes', { event: '*', table: 'orders' }, () => {})
    ch.subscribe((s) => statuses.push(s))
    await new Promise((r) => setTimeout(r, 10))
    expect(sock.sent).toContain(JSON.stringify({ type: 'subscribe', collection: 'orders' }))
    client._testInjectMessage(JSON.stringify({ type: 'subscribed', collection: 'orders' }))
    expect(statuses).toContain('SUBSCRIBED')
    await client.removeAllChannels()
  })

  it('change dispatch respects event + row filter + table', async () => {
    const { client } = clientWithFake()
    const seen: unknown[] = []
    const ch = client.channel('f')
    ch.on('postgres_changes', { event: 'INSERT', table: 'orders', filter: 'status=eq.open' }, (p) => seen.push(p))
    ch.subscribe()
    client._testInjectMessage(JSON.stringify({ type: 'change', collection: 'orders', event: 'insert', data: { status: 'open', id: 1 } }))
    client._testInjectMessage(JSON.stringify({ type: 'change', collection: 'orders', event: 'delete', data: { status: 'open' } }))
    client._testInjectMessage(JSON.stringify({ type: 'change', collection: 'orders', event: 'insert', data: { status: 'closed' } }))
    client._testInjectMessage(JSON.stringify({ type: 'change', collection: 'other', event: 'insert', data: { status: 'open' } }))
    expect(seen).toHaveLength(1)
    await client.removeAllChannels()
  })

  it('broadcast + presence dispatch to matching bindings; track updates presenceState', async () => {
    const { client } = clientWithFake()
    const ch = client.channel('room:1', { config: { presence: { key: 'u1' } } })
    const bcast: unknown[] = []
    const pres: unknown[] = []
    ch.on('broadcast', { event: 'cursor' }, (p) => bcast.push(p))
    ch.on('broadcast', { event: 'other' }, (p) => bcast.push(p))
    ch.on('presence', { event: '*' }, (p) => pres.push(p))
    ch.subscribe()
    client._testInjectMessage(JSON.stringify({ type: 'broadcast', event: 'cursor', payload: { x: 1 } }))
    client._testInjectMessage(JSON.stringify({ type: 'broadcast', event: 'ignored', payload: {} }))
    expect(bcast).toHaveLength(1)
    await ch.track({ online: true })
    expect(ch.presenceState()['u1']).toEqual({ online: true })
    expect(pres.length).toBeGreaterThan(0)
    await ch.untrack()
    expect(ch.presenceState()['u1']).toBeUndefined()
    await client.removeAllChannels()
  })

  it('error routes to channel onError + CHANNEL_ERROR; send fails when offline', async () => {
    const { client } = clientWithFake()
    const ch = client.channel('e')
    const errs: string[] = []
    const statuses: string[] = []
    ch.on('postgres_changes', { event: '*', table: 'orders' }, () => {})
    ch.onError((m) => errs.push(m))
    ch.subscribe((s) => statuses.push(s))
    client._testInjectMessage(JSON.stringify({ type: 'error', collection: 'orders', error: 'subscribe failed' }))
    expect(errs).toEqual(['subscribe failed'])
    expect(statuses).toContain('CHANNEL_ERROR')

    const offline = new RealtimeClient('ws://x/v1/realtime', {
      apikey: 'ob_testkey123456',
      wsFactory: () => {
        const s = fakeSocket()
        ;(s as { readyState: number }).readyState = 3 // CLOSED
        return s
      },
    })
    const ch2 = offline.channel('b')
    ch2.on('broadcast', { event: '*' }, () => {})
    const res = await ch2.send({ type: 'broadcast', event: 'e', payload: {} })
    expect(res.status).toBe('error')
    await client.removeAllChannels()
  })

  it('removeChannel unsubscribes + getChannels tracks', async () => {
    const { client, sock } = clientWithFake()
    const ch = client.channel('t')
    ch.on('postgres_changes', { event: '*', table: 'orders' }, () => {})
    ch.subscribe()
    await new Promise((r) => setTimeout(r, 10))
    expect(client.getChannels()).toHaveLength(1)
    await client.removeChannel(ch)
    expect(sock.sent).toContain(JSON.stringify({ type: 'unsubscribe', collection: 'orders' }))
    expect(client.getChannels()).toHaveLength(0)
  })

  it('ticket auth uses ?ticket= instead of ?apiKey=', async () => {
    let url = ''
    const c = new RealtimeClient('ws://h/v1/realtime', {
      apikey: 'ob_testkey123456',
      createTicket: () => 'single-use-ticket',
      wsFactory: (u) => {
        url = u
        const s = fakeSocket()
        queueMicrotask(() => s.onopen?.({}))
        return s
      },
    })
    c.channel('x').on('broadcast', { event: '*' }, () => {}).subscribe()
    await new Promise((r) => setTimeout(r, 10))
    expect(url).toContain('ticket=single-use-ticket')
    expect(url).not.toContain('apiKey=')
    await c.removeAllChannels()
  })

  it('onStatus callback + factory error is swallowed', async () => {
    const spy = vi.fn()
    const c = new RealtimeClient('ws://h/v1/realtime', { apikey: 'ob_testkey123456' })
    const off = c.onStatus(spy)
    off()
    expect(spy).not.toHaveBeenCalled()
  })
})
