/**
 * Engine-aware query editor configuration.
 * Every engine supports raw execution now: SQL engines run SQL directly
 * (reads + DML/DDL), FerretDB runs mongo-shell commands, Valkey runs Redis
 * commands, ArcadeDB runs its SQL, and Qdrant runs SCROLL/SEARCH/UPSERT/etc.
 */

export type EditorKind = "sql" | "mongo" | "redis" | "arcade" | "vector" | "generic";

export interface EngineEditorConfig {
  kind: EditorKind;
  languageLabel: string;
  placeholder: string;
  sample: string;
  supportsRaw: boolean;
  rawNote: string;
}

const SQL_SAMPLE = `-- Reads + writes/DDL (one statement, max 200 rows, 15s timeout)
SELECT * FROM users LIMIT 25;`;

const CONFIGS: Record<string, EngineEditorConfig> = {
  postgres: {
    kind: "sql",
    languageLabel: "PostgreSQL",
    placeholder: "SELECT * FROM users LIMIT 25;",
    sample: SQL_SAMPLE,
    supportsRaw: true,
    rawNote: "Runs directly on Postgres — SELECT plus INSERT/UPDATE/DELETE/DDL.",
  },
  mysql: {
    kind: "sql",
    languageLabel: "MySQL",
    placeholder: "SELECT * FROM users LIMIT 25;",
    sample: SQL_SAMPLE,
    supportsRaw: true,
    rawNote: "Runs directly on MySQL — SELECT plus INSERT/UPDATE/DELETE/DDL.",
  },
  ferretdb: {
    kind: "mongo",
    languageLabel: "MongoDB (FerretDB)",
    placeholder: 'db.users.find({ "status": "active" })',
    sample: `// FerretDB: mongo-shell — reads + writes (max 200 rows, 15s timeout)
db.users.find({ "status": "active" })
// db.users.insertOne({ "status": "active", "name": "Ada" })
// db.users.updateMany({ "status": "active" }, { "$set": { "tier": "pro" } })
// db.users.deleteOne({ "name": "Ada" })`,
    supportsRaw: true,
    rawNote: "Runs mongo-shell commands directly — find/insert/update/delete/drop.",
  },
  valkey: {
    kind: "redis",
    languageLabel: "Valkey (Redis)",
    placeholder: "KEYS *",
    sample: `// Valkey: raw Redis commands — reads + writes (one per line, last result shown)
KEYS *
// SET users:1 '{"name": "Ada"}'
// GET users:1
// HSET users:1 name Ada
// DEL users:1`,
    supportsRaw: true,
    rawNote: "Runs Redis commands directly — GET/SET/HSET/DEL/KEYS and more.",
  },
  arcadedb: {
    kind: "arcade",
    languageLabel: "ArcadeDB SQL",
    placeholder: "SELECT FROM Person LIMIT 25",
    sample: `-- ArcadeDB SQL — reads + writes/DDL (max 200 rows, 15s timeout)
SELECT FROM Person LIMIT 25
-- INSERT INTO Person SET name = 'Ada'
-- UPDATE Person SET tier = 'pro' WHERE name = 'Ada'
-- DELETE FROM Person WHERE name = 'Ada'`,
    supportsRaw: true,
    rawNote: "Runs ArcadeDB SQL directly — SELECT via query API, writes/DDL via command API.",
  },
  qdrant: {
    kind: "vector",
    languageLabel: "Qdrant",
    placeholder: 'SCROLL mycol {"limit": 25}',
    sample: `// Qdrant: SCROLL/SEARCH/UPSERT/DELETE/CREATE/DROP (max 200 rows, 15s timeout)
SCROLL mycol {"limit": 25}
// SEARCH mycol {"vector": [0.1, 0.2, 0.3], "limit": 5}
// UPSERT mycol [{"id": 1, "vector": [0.1, 0.2, 0.3], "payload": {"name": "Ada"}}]
// DELETE mycol {"filter": {"must": [{"key": "name", "match": "Ada"}]}}
// CREATE mycol {"vectors": {"size": 3, "distance": "Cosine"}}`,
    supportsRaw: true,
    rawNote: "Runs Qdrant SCROLL/SEARCH/UPSERT/DELETE/CREATE/DROP directly.",
  },
};

export function editorConfigFor(engine: string | null | undefined): EngineEditorConfig {
  const e = (engine ?? "").toLowerCase();
  return (
    CONFIGS[e] ?? {
      kind: "generic",
      languageLabel: "Query",
      placeholder: "SELECT * FROM users LIMIT 25;",
      sample: SQL_SAMPLE,
      supportsRaw: false,
      rawNote: "Connect a database to enable the query runner.",
    }
  );
}

export function historyKey(projectId: string): string {
  return `openbase:sql-history:${projectId}`;
}

export function loadHistory(projectId: string, limit = 20): string[] {
  try {
    const raw = localStorage.getItem(historyKey(projectId));
    const arr = raw ? JSON.parse(raw) : [];
    return Array.isArray(arr) ? arr.filter((s): s is string => typeof s === "string").slice(0, limit) : [];
  } catch {
    return [];
  }
}

export function saveHistory(projectId: string, query: string, limit = 20): string[] {
  const q = query.trim();
  if (!q) return loadHistory(projectId, limit);
  const next = [q, ...loadHistory(projectId, limit).filter((h) => h !== q)].slice(0, limit);
  try {
    localStorage.setItem(historyKey(projectId), JSON.stringify(next));
  } catch {
    // private mode etc — history is best-effort
  }
  return next;
}
