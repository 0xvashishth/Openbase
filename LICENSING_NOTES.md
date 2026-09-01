# Licensing Notes — Why These Databases Were Chosen

Kept as a standalone doc so anyone adding a new adapter later checks licensing before writing code, not after.

## The core problem

MongoDB (real MongoDB, not FerretDB) is licensed under the **Server Side Public License (SSPL)**, not an OSI-approved open source license. If a platform offers MongoDB's functionality as a hosted/managed service to third parties, the SSPL requires that platform to open source its *entire* surrounding service stack — management layer, provisioning code, dashboards, orchestration — under SSPL too. Redis made a similar move (now dual-licensed RSALv2/SSPLv1, though Redis 8 moved to AGPLv3 in 2025) which is why the community-maintained **Valkey** fork exists.

This is why the platform does not bundle or provision real MongoDB or Redis directly — it uses **FerretDB** (Mongo-wire-protocol compatible, stores data in Postgres, Apache 2.0 licensed) and **Valkey** (Redis-compatible fork, BSD licensed) instead.

## Safe-to-bundle license reference

| License | OSI Approved | Safe to redistribute/host as part of a product? |
|---|---|---|
| PostgreSQL License (BSD-style) | Yes | Yes, no restrictions |
| Apache 2.0 | Yes | Yes, no restrictions |
| BSD | Yes | Yes, no restrictions |
| MIT | Yes | Yes, no restrictions |
| GPLv2 / GPLv3 | Yes | Yes, but copyleft — modifications to the DB itself must stay open; doesn't affect *your* separate application code |
| SSPL | **No** | Only if you're not offering it "as a service" to others — risky for this platform's use case, avoid |
| BSL (Business Source License) | No | Time-delayed open source (e.g. converts to Apache after N years) — usable but check the specific conversion terms per project (e.g. CockroachDB) |

## Note on BYODB mode

The licensing concern above applies specifically to databases the **platform itself bundles/provisions**. In BYODB mode, the user is running their own MongoDB instance (or whatever else) themselves — the platform is just a client connecting to it, the same way any app with a MongoDB driver would. That does not trigger SSPL's "offering as a service" clause, since the platform isn't the one operating the MongoDB deployment.

## Action item for future adapters

Before adding any new database engine as a **provisioned** option, check its current license — some projects change licensing terms with little notice (Redis is the cautionary example here). BYODB support can be added for any engine regardless of its license, since the platform never redistributes it.
