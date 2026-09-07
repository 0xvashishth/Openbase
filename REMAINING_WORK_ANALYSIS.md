# Openbase Remaining Work Analysis

> **Status update (2026-09-07):** Tier 0 (all 4 live defects) and Phase 8
> (8.7, 8.8, 8.9, 8.10) plus 18.1 persistent volumes are now ✅ **shipped and
> tested**. `PHASES.md` carries the current per-item status. The inventory
> below remains accurate for Phases 9–18. Next up per the plan: Phase 9
> (operator identity) and 13.2/13.6 (row editing, SQL editor v2).

## Summary

This document analyzes the actual state of the Openbase codebase against its documented roadmap in PHASES.md, identifies gaps, and provides a prioritized plan for completion.

**Key Finding:** While Phases 0-6 are solidly complete and Phase 7 has 4/6 adapters implemented, **Phase 8 remediation items are only partially done**, and **Phases 9-18 are largely not started**.

## Verified Status by Phase

### Part I (Phases 0-7) - Foundation Complete

| Phase | Claimed | Actual Verdict | Notes |
|-------|---------|----------------|-------|
| 0 | ✅ complete | **DONE** | Auth, org/project management |
| 1 | ✅ complete | **DONE** | Adapter interface + Postgres adapter |
| 2 | ✅ complete | **DONE** | FerretDB adapter |
| 3 | ✅ complete | **DONE** | Visual schema explorer + auto-generated REST API |
| 4 | ✅ complete | **DONE** | Triggers + runtime functions (Node only) |
| 5 | ✅ complete | **PARTIAL** | Realtime layer (browser WS auth broken) |
| 6 | ✅ complete | **DONE** | BYODB hardening |
| 7 | 🔄 in progress | **PARTIAL** | Remaining adapters + polish (Chroma missing) |

### Phase 8 - Remediation Status

| Item | Verdict | Evidence | What Remains |
|------|---------|----------|--------------|
| **8.1 PK resolution** | **DONE** | `public_api_handlers.go:85-123` - full `resolveRowKey()` with cache | None |
| **8.2 Postgres realtime per-collection notify** | **DONE** | TG_ARGV[0] used; COALESCE(NEW,OLD); cross-talk E2E test exists | None |
| **8.3 Adapter connection pooling** | **DONE** | `internal/pool/pool.go` + `adapter_pool.go`; `GET /v1/projects/{id}` exists | None |
| **8.4 RBAC enforcement** | **DONE** | `authorizeOrgRole` middleware; RBAC matrix in SCHEMA.md; tests | None |
| **8.5 API key scopes** | **PARTIAL** | Scopes enforced; UNIQUE on `key_hash` migration 0002 | `last_used_at`, `expires_at`, display prefix (`ob_abc…`) not confirmed |
| **8.6 Org/project lifecycle** | **DONE** | PATCH/DELETE /v1/orgs/{id}, member CRUD, transfer-ownership exist | Email invites blocked on mailer (9.2) |
| **8.7 Abuse controls/operability** | **NOT STARTED** | No `/healthz`, `/readyz`, `/metrics`; no request-ID middleware; no auth rate limiting; no per-API-key quotas; docker-compose healthcheck missing | All items |
| **8.8 Function sandbox secrets** | **NOT STARTED** | `runner.go` passes `os.Environ()` through; Alpine image has no Node; no env allow-list; no per-project concurrency cap | Strip env, add allow-list, add Node to image or gate on runtime detection, add concurrency cap |
| **8.9 Webhook hardening** | **NOT STARTED** | No HMAC signature, no timeout, no SSRF deny-list, no retries, no `webhook_deliveries` table | HMAC `X-Openbase-Signature`, SSRF deny-list, timeout, exponential backoff, delivery table+UI |
| **8.10 Dashboard debt** | **PARTIAL** | Toast system exists; `ConfirmDialog` used (no `window.confirm`) | `error.tsx`/`not-found.tsx` missing; 5-file nav list duplication; no SWR/react-query; Cmd+Enter in SQL inserts blank line; 7/10 panels use local `ErrorBanner` |

## Remaining Work Inventory by Phase

### Phase 9 - Platform Accounts (NOTHING STARTED)
- **9.1** Sessions + refresh tokens (fix 24h unrevocable JWT)
- **9.2** Mailer abstraction (SMTP + templates) - **BLOCKS 9.3, 9.7, Phase 10**
- **9.3** Credential lifecycle (email verify, password reset)
- **9.4** Account settings UI (`/account` page)
- **9.5** Platform SSO (GitHub/Google OAuth)
- **9.6** Operator MFA (TOTP + recovery codes)
- **9.7** Email invites (tables exist in migration 0003 - need routes)
- **9.8** Audit log writer (seam exists via `AuditSink` + `audit_events` table)

### Phase 10 - End-User Auth (NOTHING STARTED - LARGEST GAP)
- **10.0** Architecture decision: metadata-DB vs native-auth (settle first)
- **10.1** Identity model (`project_users`, `project_identities`, per-project JWT signing keys)
- **10.2** Public auth endpoints (signup, login, refresh, logout, magic link, OTP)
- **10.3** Auth providers (email → magic link → OAuth → phone OTP → anonymous)
- **10.4** Policy + abuse surface (redirect allowlist, rate limits, captcha, password policy)
- **10.5** End-user MFA (TOTP, aal1/aal2 in JWT)
- **10.6** Auth hooks (before/after user created, before token issued)
- **10.7** Auth section in dashboard (Users table, Providers, Email templates, etc.)

### Phase 11 - Authorization/RLS (NOTHING STARTED - DEPENDS ON 10)
- **11.1** Key roles: `anon`, `authenticated`, `service_role`; JWT auth context
- **11.2** Policy model (`policies` table, AST-based expression)
- **11.3** Two-tier enforcement: Tier A native (Postgres/MySQL), Tier B platform-side
- **11.4** Default-deny, `security_enabled` switch, policy templates, "test as user" simulator
- **11.5** Single `internal/authz` package for Data API + Realtime + Storage
- **11.6** Third-party JWT acceptance (Auth0/Clerk/Firebase via JWKS)

### Phase 12 - Data API Parity + SDKs (NOTHING STARTED - DEPENDS ON 11)
- **12.1** PostgREST query grammar (`eq/neq/gt/like/in/or`, `select=`, `order=`, `Range`, `count=exact`)
- **12.2** Embedded resources via FK following (`select=*,orders(*)`)
- **12.3** Write semantics (bulk insert, upsert, PATCH, `Prefer: return=representation`)
- **12.4** RPC endpoint (`POST /v1/api/rpc/{name}`, capability-gated)
- **12.5** OpenAPI 3.1 generation + API Docs tab in dashboard
- **12.6** GraphQL API (deferred from Phase 3 stretch)
- **12.7** `@openbase/js` SDK + `openbase-py`

### Phase 13 - Table Editor + DDL (NOTHING STARTED - BIGGEST UI PHASE)
- **13.1** `SchemaMutator` interface (DDL on adapters) + identifier validation + audit logging
- **13.2** Row editing: insert panel, inline cell edit, row drawer, delete/duplicate, bulk delete, FK selector
- **13.3** Grid upgrades: total row count, page-size selector, multi-column sort, real filter builder, column persistence
- **13.4** CSV/JSON import wizard + export
- **13.5** Schema management UI (new table, column editor, index manager, enum manager, views)
- **13.6** SQL Editor v2 (Cmd+Enter, query tabs, server-stored snippets, schema-aware autocomplete, EXPLAIN with plan visualizer, export, pagination beyond 200 rows)
- **13.7** Database section pages (extensions, roles/grants, publications, DB functions, native triggers, indexes)
- **13.8** Visual schema designer becomes editable ERD; persist node layout

### Phase 14 - Storage (0% IMPLEMENTED)
- **14.1** `internal/storage` backend interface (local FS + S3-compatible)
- **14.2** `buckets` + `objects` metadata tables
- **14.3** REST surface (upload, download, HEAD, list, move, copy, signed URLs, range requests)
- **14.4** Storage policies via Phase 11 evaluator
- **14.5** TUS resumable uploads, image transformations, S3-compatible gateway
- **14.6** Storage UI (bucket list, file browser, drag-drop upload, preview, bulk ops, signed URL)

### Phase 15 - Functions v2 (MOSTLY ABSENT)
- **15.1** HTTP invocation endpoint (`POST /v1/functions/{name}`, API key or JWT auth)
- **15.2** Function lifecycle (PUT to update, version history, CodeMirror editor, test invoke panel, source viewer)
- **15.3** Per-function secrets + env allow-list (builds on 8.8)
- **15.4** `function_invocations` table, log tail UI, retries + DLQ
- **15.5** Real sandbox isolation (gVisor/Firecracker, CPU quota, network egress policy)
- **15.6** Python runtime (currently always fails at runtime)
- **15.7** Scheduled jobs (cron) - engine-agnostic, works for all 6 databases
- **15.8** Durable queues (metadata-DB-backed, enqueue/dequeue/ack/DLQ)

### Phase 16 - Realtime v2
- **16.1** Browser-usable auth (ticket endpoint or WS subprotocol - currently completely broken)
- **16.2** Postgres Changes v2 (per-subscription event filter, old_record, logical decoding)
- **16.3** Broadcast channels (client↔client, HTTP publish)
- **16.4** Presence (join/leave, per-connection state, diff-based sync)
- **16.5** Authorization through Phase 11 evaluator
- **16.6** Delivery guarantees (heartbeat/ack, message IDs, replay cursor, slow-consumer signal)
- **16.7** Polling tier for non-native engines (FerretDB, MySQL, ArcadeDB, Qdrant, Valkey)
- **16.8** Realtime inspector UI (replace broken demo)

### Phase 17 - Observability (0%)
- **17.1-17.7:** Log pipeline, Logs Explorer UI, Prometheus metrics + Reports page, query performance, Security Advisor, Performance Advisor, log drains

### Phase 18 - Ops + Leftovers
- **18.1** Persistent volumes for provisioned containers (**CRITICAL** - currently `docker rm -f` = data loss)
- **18.2** User DB migration ledger
- **18.3** Schema branching
- **18.4** Declarative schema (schema-as-files)
- **18.5** `openbase` CLI (`login`, `link`, `db pull/push`, `migration new/up`, `functions new/deploy`, `gen types`, `logs`, `start`)
- **18.6** Management API (stable OpenAPI) + Terraform provider
- **18.7** SSL/CA cert upload, broken-out connection fields, IP allow-lists, Supavisor-equivalent pool endpoint
- **18.8** Multiple databases per project (drop UNIQUE on connections.project_id)
- **18.9** Chroma adapter, vector IR, ArcadeDB graph traversal, live integration tests, adapter contribution guide
- **18.10** MCP server + AI assistant (stretch)

## Live Defects - Fix Immediately (Tier 0)

These should be addressed **this week** regardless of phase order:

1. **18.1 partial: Add persistent volumes to provisioned containers**
   - Currently `docker rm -f` destroys data irrecoverably
   - One-line change in `provision.go` + volume in `docker-compose.yml`

2. **8.8: Strip `os.Environ()` from function sandbox**
   - Platform secrets (`OPENBASE_JWT_SECRET`, `OPENBASE_ENCRYPTION_KEY`) readable by any user function
   - Replace with explicit env allow-list
   - Add Node to Docker image so function triggers don't silently no-op in production

3. **16.1 partial: Add WS query-param auth for Realtime**
   - Entire Realtime feature non-functional from browsers (apiKey accepted, never sent)
   - Accept `?token=` query parameter in `apikey_middleware.go`
   - Fix `RealtimeDemo.tsx` to send key and gate `setConnected` on `onopen`

4. **8.9: Add SSRF deny-list for webhooks**
   - `action_target` accepts `http://169.254.169.254/...` (IMDS attack vector)
   - Add SSRF block before first HTTP POST in `EndpointAction`

## Novel Features to Consider Adding

These are genuinely missing from PHASES.md and represent opportunities where Openbase's multi-engine premise enables unique value:

### Differentiators (Multi-Engine Premise)
| Feature | Why It Fits |
|---------|-------------|
| **Cross-database query federation** (join Postgres + Qdrant) | Unique capability - no BaaS offers this |
| **Multi-database transactions via saga pattern** | Postgres for metadata + Valkey for session cache - no BaaS has this |
| **Adapter marketplace / plugin registry** | Turn 6-engine layer into extensible platform |
| **Engine migration wizard** (FerretDB → Postgres, etc.) | Possible because Openbase owns both sides of adapter interface |
| **Vector + relational hybrid queries** (`WHERE embedding <-> $vec < 0.5 AND user_id = auth.uid()`) | Headline AI feature |

### Developer Experience Gaps
| Feature | Priority |
|---------|----------|
| **Type generation** (`openbase gen types` → TypeScript interfaces) | High - pull from 18.5 CLI, ship standalone |
| **Local development stack** (`openbase start`) | High - essential for community adoption |
| **Environment promotions** (dev → staging → prod with schema diff) | Medium |
| **Seed data management** (versioned JSON/SQL seeds) | Medium |
| **Function testing harness** (run locally against real DB) | High |

### Security & Compliance
| Feature | Reason |
|---------|--------|
| **API key rotation without downtime** (grace period) | Current instant revoke breaks apps |
| **Leaked-credential detection** (scan DSNs) | Self-hosted breach = catastrophic |
| **SCIM provisioning** for org management | Enterprise self-hosters need IdP-driven provisioning |
| **Data masking / column redaction** via `masked` flag on `ColumnInfo` | Novel vs Supabase, GDPR relevant |

## Prioritized Execution Plan

### Tier 0 - Fix Immediately (This Week)
1. **18.1** Add persistent volumes to provisioned containers
2. **8.8** Fix function sandbox secret leakage
3. **16.1** Enable browser WebSocket authentication for Realtime
4. **8.9** Add SSRF protection to webhooks

### Tier 1 - Complete Phase 8 (Prerequisite)
5. **8.7** Implement `/healthz`, `/readyz`, `/metrics`, request-ID middleware, auth rate limiting, API-key quotas
6. **8.10** Consolidate nav list, add error pages, add SWR/react-query, fix Cmd+Enter, migrate panels to toast

### Tier 2 - Phase 9 (Operator Identity)
7. **9.1** Sessions + refresh tokens
8. **9.2** Mailer abstraction (**unlocks 9.3, 9.7, Phase 10**)
9. **9.3 + 9.4** Email verification, password reset, account settings UI
10. **9.7** Email invites (routes only - tables exist in 0003)
11. **9.8** Audit log writer (seam already in place)
12. **9.5 + 9.6** Platform SSO + operator MFA (stretch)

### Tier 3 - Phase 15 (Functions - Can Start Early)
13. **15.1** HTTP invocation endpoint
14. **15.2** Function update (PUT) + CodeMirror editor + source viewer
15. **15.4** `function_invocations` table + log tail UI

### Tier 4 - Phase 10 (End-User Auth - Largest Piece)
16. **10.0** Architecture decision document first
17. **10.1** Identity model + JWKS endpoint
18. **10.2 + 10.3** Public auth endpoints + providers
19. **10.4 + 10.5** Policies, rate limits, MFA
20. **10.6 + 10.7** Auth hooks + dashboard Auth section

### Tier 5 - Phase 11 (Authorization - Depends on 10)
21. **11.1 + 11.2** Key role split + policy model
22. **11.3** Two-tier enforcement engine
23. **11.4** Default-deny, templates, "test as user" simulator
24. **11.5 + 11.6** `internal/authz` package + third-party JWT acceptance

### Tier 6 - Phase 13 (Table Editor - Biggest UX Win)
25. **13.2** Row editing (backend already supports POST/PUT/DELETE)
26. **13.6** SQL Editor v2 (schema already fetched - just needs UI)
27. **13.3** Grid upgrades (total count needs backend work)
28. **13.1 + 13.5** DDL interface + schema management UI (needs 9.8 audit log first)

### Tier 7 - Phase 12 (Data API Parity - Depends on 11)
29. **12.1** PostgREST query grammar
30. **12.5** OpenAPI generation + API Docs tab
31. **12.3** Bulk writes, upsert, PATCH
32. **12.7** `@openbase/js` SDK

### Tier 8 - Phase 14 + 16 (Parallel After 11)
33. **16.1-16.6** Realtime v2 (proper auth, Postgres Changes v2, broadcast, presence, delivery)
34. **14.1-14.4** Storage core (backend interface, metadata, REST surface, policies)

### Tier 9 - Phase 17 + 18 (Ops and Finish)
35. **17.1-17.6** Observability, security/performance advisors
36. **18.2-18.5** Migrations, branching, declarative schema, CLI
37. **18.7-18.9** SSL, multi-DB per project, Chroma, vector IR, ArcadeDB graph

## Schedule for New Features (From Novel Additions)

| Feature | Suggested When |
|---------|----------------|
| **Type generation** (`gen types`) | Pull out of 18.5 CLI - ship standalone after Phase 13 |
| **API key rotation with grace period** | Add to Phase 8.5 or 9 |
| **OTel export** (traces, not just metrics) | Add to Phase 17 |
| **Data masking / column redaction** | Add to Phase 11 (policy model) |
| **Cross-database federation query** | New Phase 19 - after 18.8 multi-DB |
| **Engine migration wizard** | New Phase 19 - after 13.1 DDL |
| **Admin event webhooks** | Add to Phase 15.7 scheduling |
| **Adapter marketplace** | New Phase 19 - after 18.9 contribution guide |

---

## Key Conclusions

**What's Actually Done:**
- Phases 0-6: Solid and verified
- Phase 7: 4/6 adapters done (Chroma missing)
- Phase 8: 8.1, 8.2, 8.3, 8.4, 8.6 complete; 8.5 partial; 8.7-8.10 not started

**Critical Live Defects:**
1. Provisioned containers have no volumes → data loss on delete
2. Function sandbox leaks all platform secrets to user code
3. Realtime WebSocket completely broken from browsers
4. Webhook destinations accept SSRF targets (IMDS attack)

**Largest Structural Gaps (in order of impact):**
1. End-user authentication (Phase 10) - enables RLS, user data isolation
2. Authorization/RLS equivalent (Phase 11) - makes platform usable from browsers
3. Table editor write paths (Phase 13) - biggest UI gap, but backend already supports it
4. Storage (Phase 14) - 0% implemented
5. Realtime browser auth (Phase 16.1) - completely broken

**Immediate Next Actions:**
1. Fix the 4 Tier-0 live defects above (takes <1 day total)
2. Complete Phase 8 (8.7-8.10) - prerequisite for everything else
3. Start Phase 13.2 (row editing) and 13.6 (SQL editor v2) in parallel - highest UI impact for lowest backend work since the endpoints already exist

This analysis provides a realistic foundation for planning completion of the Openbase platform based on actual code inspection rather than documentation claims.