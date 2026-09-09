# `@openbase/js` — Official JavaScript SDK for Openbase

Supabase-style DX over any Openbase project database: one `createClient(url, anonKey)`
gives you end-user auth, a PostgREST-compatible query builder, realtime channels,
and forward-compatible storage/functions.

```sh
npm install @openbase/js
```

```ts
import { createClient } from '@openbase/js'

const openbase = createClient(process.env.OPENBASE_URL!, process.env.OPENBASE_ANON_KEY!)

// Auth (requires the Phase 10 `/auth/v1/*` backend; see SDK_PLAN.md Track 1)
await openbase.auth.signUp({ email, password })
await openbase.auth.signInWithPassword({ email, password })
openbase.auth.onAuthStateChange((event, session) => console.log(event, session?.user.id))

// Database (live today over `ob_` API keys)
const { data, error } = await openbase
  .from('orders')
  .select('id,status')
  .eq('status', 'open')
  .order('created_at', { ascending: false })
  .limit(20)

// Realtime (live today for Postgres collection changes)
openbase
  .channel('orders-feed')
  .on('postgres_changes', { event: '*', table: 'orders', filter: 'status=eq.open' }, (change) => {
    console.log(change.event, change.data)
  })
  .subscribe((status) => console.log(status))

// Storage / Functions (stable surface; typed "not enabled" errors until Phases 14/15 land)
await openbase.storage.from('avatars').upload('me.png', file)
await openbase.functions.invoke('hello', { body: { name: 'Ada' } })

// Next.js SSR (`@openbase/js/server`)
import { createServerClient } from '@openbase/js/server'
const server = createServerClient(url, key, { cookies }) // { get, set, remove }
```

## Conventions

- Async methods resolve `{ data, error }` and **never throw on HTTP errors**.
  Only programmer errors (bad URL/key, `service_role` in a browser) throw.
- Capability-gated backends fail with explicit errors, never wrong answers.
- Works in browsers (native `fetch`/`WebSocket`), Node 18+ (pass `fetch`/`wsFactory`
  if your runtime lacks them), and edge runtimes.

## Development

```sh
npm install
npm test          # vitest
npm run typecheck # tsc --noEmit
npm run build     # tsup (ESM+CJS+d.ts, incl. ./server entry)
```

See `SDK_PLAN.md` (repo root) for the phase plan and backend dependencies.
