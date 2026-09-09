# Openbase JS SDK + End-User Auth Plan (`@openbase/js`)

> Goal: an installable npm package that talks to Openbase the way
> `@supabase/supabase-js` talks to Supabase and `firebase` talks to Firebase:
> `createClient(url, anonKey)` → `auth` (signup/login/session) + `from(table)`
> (data) + `channel` (realtime), with storage/functions/server-helpers reserved
> for later phases.
>
> **Ordering constraint (non-negotiable): auth backend first, SDK second.**
> Today Openbase has only *platform-operator* auth (`POST /v1/auth/*`,
> HS256, single 24h JWT in `internal/auth/auth.go`, sessions partially in
> Phase 9). There is **no end-user (app-user) identity** — see `PHASES.md`
> Part II §A. An SDK `auth.signUp/signIn` cannot ship until Phase 10
> (GoTrue equivalent) lands. This plan therefore has two tracks:
> **Track 1 = backend auth system** (Supabase Auth parity), **Track 2 = npm SDK**.
>
> Relationship to the existing roadmap:
> `PHASES.md` Phase 10 = Track 1, Phase 12.7 = Track 2 seed. This file is the
> detailed build plan for both; `PHASES.md` remains the ordering authority.
>
> **Implementation status (2026-09-08): Track 2 Phases S0–S4 are implemented in
> `sdk/js/` (`@openbase/js` v0.1.0) with 52 vitest tests green + `tsc --noEmit`
> clean + `tsup` ESM/CJS/d.ts build passing. Track 1 (A0–A3, GoTrue backend) is
> still pending — the SDK's `auth.*` targets the planned `/auth/v1/*` contract
> and is fully tested against mocks until the backend lands. Data (`from()`),
> realtime channels, `getTables/getSchema`, storage/functions stubs, and
> server helpers work against today's backend.**

## 0. Current state (what the SDK must build on / replace)

| Area | Today | Gap for SDK |
|---|---|---|
| Platform auth | `POST /v1/auth/register\|login\|forgot\|reset`, `GET/PATCH /v1/me`, sessions list/revoke (`internal/server/auth_handlers.go`, `server.go:213-230`) | Operator-only; no per-project users, no asymmetric keys, access token 24h HS256 (`internal/auth/auth.go:46-54`) |
| API-key data path | `GET/POST/PUT/DELETE /v1/api/{collection}` + `/_schema`, `GET /v1/api/tables` (`public_api_handlers.go`), key via `Authorization: Bearer ob_…` or `?apiKey=` (`apikey_middleware.go`) | Only `limit/offset/order_by`, always `SELECT *`, no filter operators, no `select=`, no bulk/upsert/PATCH, no RPC, no OpenAPI (`PHASES.md` §B) |
| Realtime | `GET /v1/realtime` WS, `?apiKey=` fallback, `dashboard/lib/realtime.ts` (113-line WS-only helper, resubscribe-on-reconnect) | No event-type/row filters, no `old_record` on DELETE, no broadcast/presence, no ticket auth, buffer silently drops at 256 (`PHASES.md` §F) |
| Dashboard client | `dashboard/lib/api.ts` (platform API, token in localStorage) + `realtime.ts` | Dashboard-only, no query builder, no session persistence/refresh, no SSR helpers |
| Mailer | `internal/mail` + dashboard SMTP settings (Phase 9.2) | Exists — SDK auth emails (verify/recovery/invite/magic-link/OTP) reuse it per-project (Phase 10.4) |

Design precedent to follow: Supabase splits `supabase-js` into
`@supabase/auth-js` (GoTrue client), `@supabase/postgrest-js` (query builder),
`@supabase/realtime-js` (channels), `@supabase/storage-js`, `@supabase/functions-js`.
We mirror that internally (`src/auth/`, `src/postgrest/`, `src/realtime/`, …)
but **publish one package** (`@openbase/js`, alias `openbase`) so install is one line.

```sh
npm install @openbase/js
```

```ts
import { createClient } from '@openbase/js'

const openbase = createClient(process.env.OPENBASE_URL!, process.env.OPENBASE_ANON_KEY!)

// Auth (Track 1 backend required)
await openbase.auth.signUp({ email, password })
await openbase.auth.signInWithPassword({ email, password })
openbase.auth.onAuthStateChange((event, session) => { /* SIGNED_IN, SIGNED_OUT, TOKEN_REFRESHED … */ })

// Data (works today over API keys; filters need Phase 12.1 grammar)
const { data, error } = await openbase.from('orders').select('*,customer(name)').eq('status', 'open').limit(20)

// Realtime (works today for Postgres collection-granularity; filters/broadcast/presence need Phase 16)
openbase.channel('orders-feed').on('postgres_changes', { event: '*', table: 'orders' }, console.log).subscribe()
```

---

## Track 1 — End-user auth system (backend, Supabase Auth parity)

Without this track the SDK's `auth` namespace has nothing to call. Maps 1:1 to
`PHASES.md` Phase 10 (+ Phase 9 sessions/mailer it builds on, Phase 11 keys/RLS it unblocks).

### Phase A0 — Decision + identity model (maps to Phase 10.0–10.1)

- [ ] **A0.1 Decide where end users live, write into `ARCHITECTURE.md`.**
  Recommendation (from `PHASES.md` 10.0): **(b) platform metadata DB keyed by
  `project_id` as default** (works on all six engines, leaves BYODB untouched)
  + **(a) `openbase_auth` schema inside Postgres/MySQL as opt-in "native auth"**
  mode behind new `SupportsNativeAuth` capability. Settle before coding.
- [ ] **A0.2 Migrations:** `project_users` (id, project_id, email/phone, password_hash,
  email_confirmed_at, phone_confirmed_at, banned_until, metadata JSONB, created_at),
  `project_identities` (provider, provider_uid, user_id), `project_sessions`
  (refresh-hash, user_agent, ip, expires/revoked), `project_auth_settings`
  (per-project mailer overrides, redirect allow-list, rate limits, password policy,
  session timeouts), `project_mfa_factors` + `project_mfa_challenges`.
- [ ] **A0.3 Per-project asymmetric signing keys** (ES256 default, RS256 option):
  `project_signing_keys` (kid, alg, public JWK, encrypted private, rotated_at),
  `GET /v1/projects/{id}/.well-known/jwks.json` (public), rotate endpoint
  (admin+). Replaces shared-HS256 pattern from `internal/auth/auth.go`.
- [ ] **A0.4 `internal/projectauth` package** (no HTTP dep): password hashing
  (bcrypt, reuse `internal/auth`), token mint/verify (access 15 min + rotating
  refresh with reuse detection, mirroring Phase 9.1 session model), `aal1/aal2` claims.
- **Done when:** migration applies cleanly; unit tests mint a token and verify it
  against the project's JWKS without the shared secret.

### Phase A1 — Public auth API, email+password + sessions (maps to Phase 10.2–10.3 first slice)

New project-scoped routes resolved from the **anon key** (not the operator JWT):

```
POST /auth/v1/signup            # email+password → user + session (or confirmation-sent)
POST /auth/v1/token?grant_type=password|refresh_token|id_token|pkce|otp|magiclink
POST /auth/v1/logout            # revoke refresh token
GET  /auth/v1/user              # Bearer end-user JWT → user
PUT  /auth/v1/user              # update metadata/password
POST /auth/v1/recover          # password reset email
POST /auth/v1/verify           # email/phone verify, invite accept
POST /auth/v1/magiclink        # passwordless email link
POST /auth/v1/otp              # phone/email OTP request + verify
```

- [ ] **A1.1 Key roles:** split `ob_` keys into `anon` (safe for browsers, gated by
  policies) / `service_role` (bypass, never shipped to browser) per Phase 11.1.
  Data + auth routers accept `apikey:` anon key or end-user JWT; JWT claims become
  the authorization context.
- [ ] **A1.2 Rate limits + abuse:** per-project limits, redirect-URL allow-list,
  password strength policy, leaked-password check (offline list, optional),
  session idle + absolute timeouts.
- [ ] **A1.3 Dashboard Authentication section skeleton:** Users table (search,
  create, delete, ban, confirm email, view sessions) — full Providers/Templates/Keys
  UI arrives in A3.
- **Done when:** `curl` signup → verify (via `mail_log`) → sign-in → refresh →
  `GET /auth/v1/user` works end-to-end against a real project; stolen refresh
  token reuse is detected and the chain revoked.

### Phase A2 — Providers: OAuth, OIDC, phone, anonymous (maps to Phase 10.3)

Order: magic link → GitHub/Google OAuth (PKCE, `state`+`code_verifier`) → generic
OIDC connector (covers "custom providers") → phone/SMS OTP behind pluggable
`SMSProvider` interface (Twilio/MessageBird/stub) → anonymous sign-in
(auto-convert to permanent on `linkIdentity`).

- [ ] Per-provider config in `project_auth_settings` + `project_identities` linking.
- [ ] `POST /auth/v1/authorize` (redirect out) + `GET /auth/v1/callback` (exchange,
  link-or-create, redirect back to allow-listed URL).
- [ ] Captcha hook (hCaptcha/Turnstile) on signup/OTP endpoints.
- **Done when:** browser OAuth round-trip (GitHub) and SMS-OTP (stub provider in
  tests, real provider in staging) both yield a verifiable end-user JWT.

### Phase A3 — MFA, hooks, admin API, dashboard (maps to Phase 10.5–10.7)

- [ ] **A3.1 End-user MFA:** TOTP enrol/challenge/verify, recovery codes,
  `aal1/aal2` in claims, step-up for sensitive ops.
- [ ] **A3.2 Auth hooks:** invoke project function on `before-user-created`,
  `after-user-created`, `before-token-issued` (custom claims) — reuses Phase 15
  function runtime; fail-open vs fail-closed is per-hook config.
- [ ] **A3.3 Admin API** (service_role only): list/search users, invite, ban,
  reset password, delete, **impersonate** (short-lived scoped token, `admin`+ only,
  always audit-logged via 9.8 sink).
- [ ] **A3.4 Dashboard Authentication group:** Users / Providers (redirect-URI copy
  blocks) / Email templates (variable ref + preview) / URL config / Rate limits /
  Sessions / Signing keys (view/rotate JWKS).
- **Done when:** operator can invite → ban → impersonate a user from the dashboard
  with every step in the audit log; TOTP challenge gates `aal2` routes.

---

## Track 2 — The npm package (`@openbase/js`)

Lives in `sdk/js/` (monorepo first; split to its own repo only when release
cadence diverges). TypeScript, ESM+CJS dual build, zero runtime deps except
`ws`-shim-free native WebSocket + `fetch`. Dashboard becomes first consumer
(replaces `dashboard/lib/realtime.ts`; platform calls in `dashboard/lib/api.ts`
stay separate — SDK is the *project/app* client, not the operator client).

### Phase S0 — Scaffold + transport + release train ✅ implemented (`sdk/js/`, 52 tests green)

- [ ] **S0.1 Repo layout:**

```
sdk/js/
  src/
    index.ts          # createClient + re-exports
    client.ts         # OpenbaseClient (auth+db+realtime composition)
    lib/              # fetch wrapper, errors, types, retry, storage adapters
    auth/             # AuthClient (mirrors gotrue-js surface)
    postgrest/        # query builder (mirrors postgrest-js surface)
    realtime/         # channel client (mirrors realtime-js surface)
    storage/ functions/ # stubs throwing "not yet available" (Phases 14/15)
  test/               # vitest: unit + integration (real API in Docker)
  examples/           # vite vanilla, nextjs-SSR, realtime-list
  package.json tsup.config.ts README.md CHANGELOG.md LICENSE
```

- [ ] **S0.2 `createClient(url, key, options)`:**

```ts
createClient(url, anonKey, {
  auth: { persistSession: true, autoRefreshToken: true, storageKey: 'ob-auth', detectSessionInUrl: true },
  db: { schema: 'public' },
  realtime: { params: { eventsPerSecond: 10 } },
  global: { headers: { 'x-app': 'myapp' }, fetch },
})
```

URL/key validation with actionable errors (trailing-slash trim, `ob_` prefix check).
- [ ] **S0.3 Transport:** single `fetch` wrapper (JSON encode, `Prefer`/`Range`
  passthrough, `x-request-id` echo, retry once on 502/503/504 with jitter, timeout
  via AbortController), `PostgrestError`-style typed errors `{ message, code,
  details, hint, status }`, never-throw-on-HTTP-error convention (`{ data, error }`).
- [ ] **S0.4 Session storage adapters:** `localStorage` (default, web),
  in-memory (Node), cookie adapter interface (Next.js SSR arrives in S4),
  `ReactNativeAsyncStorage` passthrough.
- [ ] **S0.5 CI:** `vitest` + `tsc --noEmit` + `eslint`, build via `tsup`
  (ESM+CJS+d.ts), Changesets, publish on tag to npm with provenance, bundle-size
  budget (<30 kB gzipped core without realtime).
- **Done when:** `npm pack` produces a typed package; integration test hits
  `GET /v1/api/tables` with an `ob_` key through the SDK transport.

### Phase S1 — Auth client (GoTrue parity; needs Track 1 A1–A3) ✅ implemented (mock-tested; live E2E waits on Track 1)

Mirror `gotrue-js` method names so Supabase docs translate 1:1:

```ts
// Email/password + magic link + OTP + OAuth + anon
signUp({ email, password, options }) → { data: { user, session }, error }
signInWithPassword({ email, password })
signInWithOtp({ email } | { phone })
verifyOtp({ email|phone, token, type })  // signup|recovery|magiclink|sms|email_change
signInWithOAuth({ provider: 'github'|'google'|'oidc', options: { redirectTo, scopes } })
signInAnonymously() → linkIdentity() on upgrade
signInWithIdToken({ provider, token })   // native Google/Apple sign-in
getSession() / getUser() / refreshSession() / signOut() / resetPasswordForEmail()
updateUser({ email|password|data })      // re-auth where required
mfa: { enroll({ factorType:'totp' }), challenge(), verify(), unenroll(), listFactors() }
admin: { listUsers, createUser, deleteUser, inviteUserByEmail, generateLink, impersonate } // service_role only
onAuthStateChange(cb) // SIGNED_IN|SIGNED_OUT|TOKEN_REFRESHED|USER_UPDATED|PASSWORD_RECOVERY|MFA_CHALLENGE_VERIFIED
```

- [ ] PKCE flow with `detectSessionInUrl` (code → session on redirect back).
- [ ] Auto-refresh loop (refresh 60 s before expiry, single-flight, tab-leader via
  `storage` event + lock), persistence + `TOKEN_REFRESHED` broadcast.
- [ ] Framework adapters deferred to S4; S1 ships vanilla + React `useSession` helper.
- **Done when:** example app signs up → confirms → signs in → refreshes across
  reload → MFA-enrols → signs out, all through the SDK, with events firing.

### Phase S2 — Database query builder (PostgREST parity; needs Phase 12.1–12.3) ✅ implemented (full grammar serialized; id-addressed fast path works on today's backend)

```ts
openbase.from('orders')
  .select('id,status,customer(name)', { count: 'exact' })
  .eq('status', 'open').neq('archived', true).gt('total', 100)
  .in('region', ['eu','us']).like('note', '%rush%').or('a.eq.1,b.gt.2')
  .order('created_at', { ascending: false, nullsFirst: false })
  .range(0, 19).limit(20).single()   // .maybeSingle() variant
await openbase.from('orders').insert([{…}], { count: 'exact' })
await openbase.from('orders').upsert(row, { onConflict: 'id', ignoreDuplicates: false })
await openbase.from('orders').update({ status: 'shipped' }).eq('id', 7)
await openbase.from('orders').delete().eq('id', 7)
await openbase.rpc('top_customers', { limit: 5 })
```

- [ ] Filter builder: full operator set (`eq/neq/gt/gte/lt/lte/like/ilike/in/is/
  not/cs/cd/sl/sr/ov/or/and`), correct URL-encoding, `select=` projection,
  `order=` multi-col, `limit/offset/range`, `Prefer: count=exact|planned|estimated`
  → `Content-Range` parsing, `return=representation|minimal`.
- [ ] Parser compiles to the existing `UniversalQuery` IR server-side so all six
  adapters inherit it; SDK surfaces capability errors verbatim
  (e.g. Qdrant `select=*,orders(*)` → explicit "embedded resources unsupported").
- [ ] Embedded resources `select=*,orders(*)`: one query per level, IN-batch
  (never N+1), gated on `SupportsForeignKeys`.
- [ ] Write semantics: bulk insert, upsert (`Prefer: resolution=merge-duplicates`),
  `PATCH` partial update, column masking from Phase 11 policies.
- **Done when:** the Phase 12 acceptance query
  `openbase.from('orders').select('*,customer(name)').eq('status','open').limit(20)`
  returns only policy-permitted rows on Postgres and an explicit capability error
  on Qdrant — driven entirely through the SDK.

### Phase S3 — Realtime client (needs Phase 16.1–16.7) ✅ implemented (gateway-compatible + client-side event/row filters; broadcast/presence forward-compat)

```ts
const ch = openbase.channel('room:1', { config: { broadcast: { ack: true }, presence: { key: uid } } })
ch.on('postgres_changes', { event: 'INSERT'|'UPDATE'|'DELETE'|'*', table: 'orders', filter: 'status=eq.open' }, cb)
  .on('broadcast', { event: 'cursor' }, cb)
  .on('presence', { event: 'sync'|'join'|'leave' }, cb)
  .subscribe((status) => { /* SUBSCRIBED|TIMED_OUT|CHANNEL_ERROR|CLOSED */ })
await ch.send({ type: 'broadcast', event: 'cursor', payload: { x, y } })
await ch.track({ online: true }); await ch.untrack()
openbase.removeChannel(ch); openbase.removeAllChannels(); openbase.getChannels()
```

- [ ] Short-lived **ticket auth** (`POST /v1/realtime/ticket` → single-use token in
  query string) replacing the long-lived `?apiKey=` WS hack from Phase 5/8.
- [ ] postgres_changes with event-type + row-filter grammar (reuses S2 filters),
  `old_record` support, heartbeat/ack, message ids, `since` replay cursor,
  explicit slow-consumer signal (replaces silent 256-drop).
- [ ] Broadcast (client↔client + HTTP publish) + presence (join/leave/state/diff-sync),
  all authorized through the Phase 11 evaluator.
- [ ] Polling-tier label passthrough: FerretDB/MySQL/etc. changes arrive tagged
  `mode: 'polling', lagMs` — honest near-realtime per the capability rule.
- **Done when:** two browser example clients exchange broadcast + presence; a
  Postgres row change arrives filtered by event+row predicate; FerretDB changes
  arrive labelled near-realtime; unauthorized subscriber receives nothing.

### Phase S4 — Server helpers + storage/functions forward-compat ✅ implemented (`@openbase/js/server`, stable stubs)

- [ ] **`@openbase/ssr`-style helpers** (in-package `server/` entry, no new dep):
  `createServerClient(cookieStore)` for Next.js App Router (read/write cookies in
  Route Handlers/Server Components), `createBrowserClient`, middleware session
  refresh pattern — mirrors `@supabase/ssr`.
- [ ] **Storage stubs with stable surface** (real backend is Phase 14):
  `from(bucket).upload/download/list/move/copy/remove/createSignedUrl` —
  implemented against the Phase 14 REST contract now, throwing
  `OpenbaseError('storage not enabled on this server')` on 404 so apps written
  today work unmodified when storage lands.
- [ ] **Functions stubs** (real backend is Phase 15): `functions.invoke(name,
  { body, headers })` → `POST /v1/functions/{name}` with anon/user-JWT auth,
  sync + fire-and-forget modes.
- **Done when:** Next.js example does cookie-based SSR (`getUser()` in a Server
  Component, refresh in middleware); storage/functions calls compile and fail
  with the documented error until their phases land.

### Phase S5 — Docs, types, dogfood, release

- [ ] Type generation: `npx openbase gen types --project <id> > db.ts` (needs Phase
  18.5 CLI contract; interim: `GET /v1/api/` OpenAPI 3.1 doc from Phase 12.5) +
  `createClient<Database>(…)` generics so `from('orders').select()` is typed.
- [ ] Typedoc + guides (getting started, auth flows incl. OAuth redirect wiring,
  RLS/policy testing via "test as user", realtime channels, SSR, E2E with Docker),
  parity table vs `supabase-js`/`firebase` per method.
- [ ] **Dashboard dogfooding:** dashboard realtime demo migrates to the SDK
  (deletes `dashboard/lib/realtime.ts`); Connect tab copy-paste snippet emits SDK
  code instead of raw `fetch`.
- [ ] 1.0 release gate: API freeze, semver policy, changelog, migration guide from
  raw REST/`?apiKey=` usage, bundle-size + browser-matrix (Chrome/FF/Safari/Edge,
  Node 18/20/22) green.
- **Done when:** fresh `npm i @openbase/js` + copy-paste quickstart builds a
  login-walled live list in <10 min against a self-hosted Openbase.

---

## Cross-cutting requirements (all phases)

1. **Capability honesty in the client:** every engine-gated call resolves to a
   typed `CapabilityError` naming the engine + missing capability, never a wrong
   answer. The SDK reads `capabilities` from `GET /v1/projects/{id}/schema` /
   `connect-info` and can `await openbase.capabilities()` to pre-gate UI.
2. **Security defaults:** anon key only in browsers; `service_role` import throws
   in browser bundles (build-time `typeof window` guard); redirect allow-list
   enforced server-side; no token in query strings except single-use realtime
   ticket; request log never records `?apiKey=`.
3. **Testing:** unit (builders produce exact query strings), contract (against
   PostgREST grammar fixtures), integration (real Postgres + FerretDB in Docker,
   per `PHASES.md` ground rule 5), browser (Playwright OAuth + realtime two-client).
4. **Versioning:** SDK major follows breaking API changes; minor tracks additive
   backend features with `serverVersion()` negotiation + `minServerVersion` guard.

## Suggested execution order

```
A0 → A1 → S0 → S1 → S2 → S3 → S4 → S5
 │     │          (SDK auth needs A1; query builder needs 12.1+11; realtime needs 16)
 └ Phase 8 remediation first (pooling/scopes/RBAC from PHASES.md §8) — S2/S3 are untrustworthy without it.
A2/A3 interleave with S1 (OAuth/MFA E2E need both sides).
Phases 14/15 (storage/functions) fill in S4 stubs when they land.
```

## Open questions for you (non-blocking — defaults assumed until you say otherwise)

1. Package name/scope: `@openbase/js` (assumed) vs `openbase` vs `@openbase/supabase-compat`?
2. Monorepo `sdk/js/` (assumed) vs separate `openbase-js` repo from day one?
3. Auth storage default for React Native (async-storage adapter included in S0?)?
4. Phone/SMS provider for Phase A2 (Twilio first, or stub-only for V1)?
5. Do you want a Supabase-drop-in compat shim (`supabase.auth.*` → `openbase.auth.*`) for migrations?
