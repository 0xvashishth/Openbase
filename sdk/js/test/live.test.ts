/**
 * Live SDK↔backend E2E (Track 2 S5, newly unblocked by Track 1).
 *
 * Boots a real stack — postgres:16-alpine in Docker + the Go API built from
 * this repo — seeds an org/project/BYODB connection via the operator API,
 * then drives the actual published SDK surface end-to-end:
 * auth (signup/login/refresh/update/MFA/signout), data (CRUD), realtime
 * (live change delivery), JWKS, and service_role admin.
 *
 * Hermetic by default: skipped unless OPENBASE_E2E=1. Run with:
 *   OPENBASE_E2E=1 npm run test:live
 * Requires: docker, go toolchain, ports 5433 + 18080 free.
 */
import { execFile, spawn, type ChildProcess } from 'node:child_process'
import { createHmac } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'

import { createClient, type OpenbaseClient } from '../src/index.js'

const run = promisify(execFile)
const LIVE = process.env.OPENBASE_E2E === '1'

const PG_CONTAINER = 'ob-e2e-pg'
const PG_PORT = '5433'
const PG_PASS = 'e2epass'
const API_PORT = '18080'
const API = `http://127.0.0.1:${API_PORT}`
// test/live.test.ts → test/ → js/ → sdk/ → repo root (three levels).
const REPO_ROOT = new URL('../../..', import.meta.url).pathname.replace(/\/$/, '')
const API_BIN = join(tmpdir(), 'ob-e2e-api')

let apiProc: ChildProcess | null = null

async function sh(file: string, args: string[]): Promise<{ stdout: string; stderr: string }> {
  return run(file, args, { timeout: 120_000 })
}

async function waitFor(fn: () => Promise<boolean>, timeoutMs: number, label: string): Promise<void> {
  const start = Date.now()
  for (;;) {
    if (await fn().catch(() => false)) return
    if (Date.now() - start > timeoutMs) throw new Error(`timed out waiting for ${label}`)
    await new Promise((r) => setTimeout(r, 1000))
  }
}

/** Minimal operator-API helper (plain fetch, Bearer operator token when set). */
function authed(token: string) {
  return async (method: string, path: string, body?: unknown) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`
    const res = await fetch(`${API}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const js = (await res.json().catch(() => null)) as Record<string, unknown> | null
    if (!res.ok) throw new Error(`${method} ${path} → ${res.status}: ${JSON.stringify(js)}`)
    return js as Record<string, unknown>
  }
}

/** RFC 6238 TOTP (SHA1, 30 s, 6 digits) for the MFA leg. */
function totpNow(secretB32: string): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const clean = secretB32.toUpperCase().replace(/=+$/, '')
  let bits = ''
  for (const ch of clean) {
    const v = alphabet.indexOf(ch)
    if (v < 0) throw new Error('bad base32')
    bits += v.toString(2).padStart(5, '0')
  }
  const bytes: number[] = []
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 1000 / 30)))
  const mac = createHmac('sha1', Buffer.from(bytes)).update(counter).digest()
  const offset = mac[mac.length - 1]! & 0x0f
  const code =
    ((mac[offset]! & 0x7f) << 24) | (mac[offset + 1]! << 16) | (mac[offset + 2]! << 8) | mac[offset + 3]!
  return String(code % 1_000_000).padStart(6, '0')
}

function jwtAal(token: string): string | undefined {
  const payload = JSON.parse(Buffer.from(token.split('.')[1]!, 'base64url').toString()) as { aal?: string }
  return payload.aal
}

describe.skipIf(!LIVE)('live SDK↔backend E2E', () => {
  let anonKey = ''
  let svcKey = ''
  let projectId = ''
  let client: OpenbaseClient

  beforeAll(async () => {
    // 1. Postgres (metadata + userdb in one container).
    await sh('docker', ['rm', '-f', PG_CONTAINER]).catch(() => undefined)
    await sh('docker', [
      'run', '-d', '--rm', '--name', PG_CONTAINER,
      '-e', `POSTGRES_PASSWORD=${PG_PASS}`, '-p', `${PG_PORT}:5432`, 'postgres:16-alpine',
    ])
    await waitFor(async () => {
      await sh('docker', ['exec', PG_CONTAINER, 'pg_isready', '-U', 'postgres'])
      return true
    }, 90_000, 'postgres')
    // pg_isready can succeed over TCP before the socket psql wants exists —
    // retry until the server is fully up. Existence-checked so a retry after
    // a half-created pair doesn't fail on "already exists".
    const ensureDb = (name: string) =>
      waitFor(async () => {
        const { stdout } = await sh('docker', [
          'exec', PG_CONTAINER, 'psql', '-U', 'postgres', '-tAc',
          `SELECT 1 FROM pg_database WHERE datname='${name}'`,
        ])
        if (stdout.trim() !== '1') {
          await sh('docker', ['exec', PG_CONTAINER, 'psql', '-U', 'postgres', '-c', `CREATE DATABASE ${name}`])
        }
        return true
      }, 90_000, `database ${name}`)
    await ensureDb('openbase')
    await ensureDb('userdb')

    // 2. Build + boot the API from this repo.
    await run('go', ['build', '-o', API_BIN, './cmd/server'], { cwd: REPO_ROOT, timeout: 300_000 })
    apiProc = spawn(API_BIN, [], {
      env: {
        ...process.env,
        OPENBASE_ADDR: `127.0.0.1:${API_PORT}`,
        OPENBASE_DATABASE_URL: `postgres://postgres:${PG_PASS}@127.0.0.1:${PG_PORT}/openbase?sslmode=disable`,
        OPENBASE_JWT_SECRET: 'e2e-test-secret-with-enough-entropy',
        OPENBASE_ENCRYPTION_KEY: 'e2e-test-master-secret-with-enough-entropy',
        OPENBASE_PUBLIC_URL: API,
      },
      stdio: 'ignore',
    })
    await waitFor(async () => {
      const r = await fetch(`${API}/healthz`)
      return r.ok
    }, 60_000, 'api /healthz')

    // 3. Seed: operator → org → project → BYODB → table → keys.
    const call = authed('')
    const reg = (await call('POST', '/v1/auth/register', {
      email: 'e2e@example.com', password: 'long-enough-password',
    })) as { token: string }
    const op = authed(reg.token)
    const org = (await op('POST', '/v1/orgs', { name: 'E2E', slug: `e2e-${Date.now()}` })) as { id: string }
    const proj = (await op('POST', `/v1/orgs/${org.id}/projects`, { name: 'Shop', slug: 'shop' })) as { id: string }
    projectId = proj.id
    await op('POST', `/v1/projects/${projectId}/connections`, {
      connection_string: `postgres://postgres:${PG_PASS}@127.0.0.1:${PG_PORT}/userdb?sslmode=disable`,
      mode: 'byodb',
    })
    await op('POST', `/v1/projects/${projectId}/sql`, {
      query: 'CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT NOT NULL, price INT NOT NULL DEFAULT 0)',
    })
    const anon = (await op('POST', `/v1/projects/${projectId}/api-keys`, { name: 'e2e-anon', role: 'anon' })) as {
      plaintext: string
    }
    const svc = (await op('POST', `/v1/projects/${projectId}/api-keys`, { name: 'e2e-svc' })) as { plaintext: string }
    anonKey = anon.plaintext
    svcKey = svc.plaintext
    client = createClient(API, anonKey)
  }, 420_000)

  afterAll(async () => {
    apiProc?.kill('SIGTERM')
    apiProc = null
    await sh('docker', ['rm', '-f', PG_CONTAINER]).catch(() => undefined)
  })

  it('auth: signup → session → user → refresh → update → signout', async () => {
    const events: string[] = []
    client.auth.onAuthStateChange((e) => events.push(e))
    const signUp = await client.auth.signUp({ email: 'shopper@example.com', password: 'long-enough-password' })
    expect(signUp.error).toBeNull()
    expect(signUp.data.session?.access_token).toBeTruthy()

    const me = await client.auth.getUser()
    expect(me.error).toBeNull()
    expect(me.data.user?.email).toBe('shopper@example.com')

    const login = await client.auth.signInWithPassword({ email: 'shopper@example.com', password: 'long-enough-password' })
    expect(login.error).toBeNull()

    const refreshed = await client.auth.refreshSession()
    expect(refreshed.error).toBeNull()
    expect(refreshed.data.session?.refresh_token).toBeTruthy()

    const updated = await client.auth.updateUser({ data: { tier: 'pro' } })
    expect(updated.error).toBeNull()

    expect(events).toContain('SIGNED_IN')

    const out = await client.auth.signOut()
    expect(out.error).toBeNull()
    const gone = await client.auth.getSession()
    expect(gone.data.session).toBeNull()
  })

  it('auth: MFA enroll → challenge → verify yields aal2', async () => {
    await client.auth.signInWithPassword({ email: 'shopper@example.com', password: 'long-enough-password' })
    const enroll = await client.auth.mfa.enroll({ factorType: 'totp' })
    expect(enroll.error).toBeNull()
    const factorId = enroll.data!.id
    const secret = enroll.data!.totp!.secret

    const list = await client.auth.mfa.listFactors()
    expect(list.data?.totp.length).toBe(1)

    const ch = await client.auth.mfa.challenge({ factorId })
    expect(ch.error).toBeNull()
    const verify = await client.auth.mfa.verify({ factorId, challengeId: ch.data!.id, code: totpNow(secret) })
    expect(verify.error).toBeNull()
    // The server issues a full aal2 session on verify; the SDK surfaces the
    // tokens without rotating local state, so assert on the returned pair.
    expect(verify.data?.access_token).toBeTruthy()
    expect(jwtAal(verify.data!.access_token)).toBe('aal2')

    const unenroll = await client.auth.mfa.unenroll({ factorId })
    expect(unenroll.error).toBeNull()
  })

  it('data: full CRUD through the query builder', async () => {
    const inserted = await client.from('items').insert({ name: 'Widget', price: 99 })
    expect(inserted.error).toBeNull()

    const selected = await client.from('items').select('id,name,price').eq('name', 'Widget').limit(10)
    expect(selected.error).toBeNull()
    const rows = selected.data as Array<{ id: number; name: string }>
    expect(rows.length).toBeGreaterThanOrEqual(1)
    const id = rows[0]!.id

    const updated = await client.from('items').update({ price: 149 }).eq('id', id)
    expect(updated.error).toBeNull()

    const check = await client.from('items').select('*').eq('id', id).single()
    expect((check.data as { price: number }).price).toBe(149)

    const deleted = await client.from('items').delete().eq('id', id)
    expect(deleted.error).toBeNull()

    const tables = await client.getTables()
    expect(tables.data?.some((t) => t.name === 'items')).toBe(true)
    const schema = await client.getSchema('items')
    expect(schema.error).toBeNull()
  })

  it('realtime: insert delivery over the channel', { timeout: 60_000 }, async () => {
    const statuses: string[] = []
    const errors: unknown[] = []
    const seen = await new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`no realtime change in 15s (statuses=${statuses.join(',')}, errors=${JSON.stringify(errors)})`)), 15_000)
      const ch = client.channel('e2e-feed')
      ch.on('postgres_changes', { event: 'INSERT', table: 'items' }, (change) => {
        clearTimeout(timer)
        resolve(change.data)
      })
      ch.onError((m) => {
        errors.push(m)
      })
      ch.subscribe((s) => {
        statuses.push(s)
      })
      // Give the socket a moment, then write through the SDK.
      setTimeout(() => {
        void client
          .from('items')
          .insert({ name: 'Live', price: 1 })
          .then((r) => {
            if (r.error) {
              clearTimeout(timer)
              reject(new Error(`live insert failed: ${JSON.stringify(r.error)}`))
            }
          })
      }, 1500)
    })
    expect((seen as { name: string }).name).toBe('Live')
    await client.removeAllChannels()
  })

  it('jwks + admin service_role surface', async () => {
    const jwksRes = await fetch(`${API}/v1/projects/${projectId}/.well-known/jwks.json`)
    expect(jwksRes.ok).toBe(true)
    const jwks = (await jwksRes.json()) as { keys: Array<{ kty: string }> }
    expect(jwks.keys[0]?.kty).toBe('EC')

    const admin = createClient(API, svcKey)
    const listed = await admin.auth.admin.listUsers()
    expect(listed.error).toBeNull()
    expect(listed.data!.users.length).toBeGreaterThanOrEqual(1)

    const created = await admin.auth.admin.createUser({ email: 'managed-e2e@example.com', email_confirm: true })
    expect(created.error).toBeNull()
    const removed = await admin.auth.admin.deleteUser(created.data!.user!.id)
    expect(removed.error).toBeNull()
  })
})
