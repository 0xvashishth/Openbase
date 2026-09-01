# Architecture & Tech Stack

## 1. System Overview

```
                         ┌─────────────────────────┐
                         │      Web Dashboard        │
                         │  (Org/Project mgmt, table  │
                         │   editor, trigger builder) │
                         └────────────┬───────────────┘
                                      │ HTTPS/REST/GraphQL
                         ┌────────────▼───────────────┐
                         │        API Gateway          │
                         │  (auth, routing, rate-limit) │
                         └────────────┬───────────────┘
                                      │
              ┌───────────────────────┼───────────────────────┐
              │                       │                       │
     ┌────────▼────────┐   ┌──────────▼─────────┐   ┌─────────▼─────────┐
     │  Core Platform    │   │  Adapter Engine      │   │  Function Runtime   │
     │  Service           │   │  (per-DB adapters)   │   │  (user code exec)   │
     │  - Auth            │   │  - Postgres adapter  │   │  - sandboxed         │
     │  - Org/Project mgmt│   │  - FerretDB adapter  │   │    execution         │
     │  - Metadata store   │   │  - Valkey adapter    │   │  - triggered by      │
     │  (own Postgres DB)  │   │  - ...               │   │    events            │
     └────────┬────────┘   └──────────┬─────────┘   └─────────┬─────────┘
              │                       │                       │
     ┌────────▼────────┐   ┌──────────▼─────────┐             │
     │  Secrets Vault    │   │  Provisioned DBs     │◄────────────┘
     │  (BYODB creds,     │   │  (Docker containers   │
     │   encrypted)        │   │   per project)         │
     └─────────────────┘   └──────────────────────┘
```

## 2. Tech Stack Recommendation

### 2.1 Core backend language: **Go**

For the pieces that matter most for performance — the Adapter Engine and API Gateway — use **Go**. Reasoning:

- Excellent concurrency model (goroutines) for handling many simultaneous DB connections across many projects/adapters — this is the actual hot path of the whole system.
- Compiles to a single static binary → trivial to ship as a Docker image, which matters a lot since V1 is self-hosted/Docker-first.
- Strong ecosystem of official/community drivers for Postgres (`pgx`), MongoDB-wire-protocol tools, Redis-compatible clients (works for Valkey), gRPC, etc.
- Much lower memory footprint than a Node.js equivalent, which matters when the same box is also running one or more provisioned database containers.
- Easier to reason about long-running low-level connections (realtime listeners, replication slots) than in a garbage-collector-heavy runtime like Node.

**Alternative considered: Rust.** Rust would be even faster and more memory-safe, but the ecosystem maturity for multiple database drivers plus development velocity favors Go for getting a v1 shipped. If raw performance of the adapter layer becomes the bottleneck later, performance-critical adapters can be rewritten in Rust and exposed via FFI/gRPC without touching the rest of the system — the adapter interface makes this swappable by design.

### 2.2 API layer: Go, exposing REST + GraphQL

- REST for the auto-generated CRUD API (mirrors what Supabase/PostgREST does).
- GraphQL as an optional layer generated from the same schema introspection, since some developers will expect it (Appwrite/Nhost users especially).

### 2.3 Web Dashboard: **TypeScript + Next.js (React)**

- Dashboard is not the performance-critical path — developer velocity and UI ecosystem matter more here.
- Next.js gives you server-rendered pages for fast initial load plus a rich client-side app for the interactive table editor / trigger builder.
- Use a canvas or SVG-based library (e.g. React Flow) for the visual schema/relationship diagram (tables, PK/FK lines).

### 2.4 Function Runtime (serverless functions feature)

- Run user functions in isolated, sandboxed containers (e.g. Firecracker microVMs or gVisor-sandboxed Docker containers) rather than in-process — this is a security boundary, not just a performance one.
- Support multiple languages from day one if possible (Node.js and Python at minimum) — this was a specific weakness called out in Supabase (TypeScript-only functions), and being multi-language is a genuine differentiator.

### 2.5 Platform's own metadata store: **PostgreSQL**

The platform needs its own database to track users, organizations, projects, and connection configs — independent of whatever database each *project* uses. Use Postgres for this (see `SCHEMA.md`). This is a fixed internal choice, not something the end user picks.

### 2.6 Secrets management

- Store BYODB credentials and provisioned-DB credentials encrypted at rest.
- Use a dedicated secrets approach (e.g. HashiCorp Vault, or at minimum envelope encryption with a KMS-style key) rather than storing plaintext or simply-encrypted values directly in the metadata Postgres DB.

### 2.7 Containerization / Deployment

- Ship as a `docker-compose.yml` for V1 (platform services + metadata Postgres + example provisioned DB).
- Each provisioned project database runs as its own container, one container per project per the "dedicated instance" model already decided on.
- Design the container orchestration piece behind an interface too (`ProvisionerInterface`) so Docker Compose can later be swapped for Kubernetes without a rewrite, when/if a cloud-hosted multi-tenant version is built.

## 3. Why not Node.js/TypeScript for the whole backend?

It's a completely viable alternative and would maximize "one language across the whole stack" simplicity (dashboard + backend both TS). The tradeoff is concurrency/performance headroom for the adapter engine specifically, which is the part of the system most likely to become a bottleneck as more databases and more concurrent projects are added. If team familiarity with Go is a concern, a reasonable middle path is: **dashboard + API gateway in TypeScript/Node, Adapter Engine in Go as a separate service communicating via gRPC.** This still isolates the performance-critical piece.
