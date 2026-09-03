/**
 * Engine-aware query editor configuration.
 * SQL engines get raw execution; NoSQL engines get the platform query
 * builder with a native-syntax preview (honest: no fake raw execution).
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

const SQL_SAMPLE = `-- Read-only: SELECT / WITH / EXPLAIN (max 200 rows, 15s timeout)
SELECT * FROM users LIMIT 25;`;

const CONFIGS: Record<string, EngineEditorConfig> = {
  postgres: {
    kind: "sql",
    languageLabel: "PostgreSQL",
    placeholder: "SELECT * FROM users LIMIT 25;",
    sample: SQL_SAMPLE,
    supportsRaw: true,
    rawNote: "Runs directly on Postgres (read-only).",
  },
  mysql: {
    kind: "sql",
    languageLabel: "MySQL",
    placeholder: "SELECT * FROM users LIMIT 25;",
    sample: SQL_SAMPLE,
    supportsRaw: true,
    rawNote: "Runs directly on MySQL (read-only).",
  },
  ferretdb: {
    kind: "mongo",
    languageLabel: "MongoDB (FerretDB)",
    placeholder: '{ "status": "active" }',
    sample: `// FerretDB: Mongo-wire preview: db.users.find({ status: "active" })
// Run executes via the platform query API — edit the JSON filter below.
{ "status": "active" }`,
    supportsRaw: false,
    rawNote: "Raw mongo shell isn't executed server-side yet — use collection + filter JSON.",
  },
  valkey: {
    kind: "redis",
    languageLabel: "Valkey (Redis)",
    placeholder: '{ "status": "active" }',
    sample: `// Valkey: collections are JSON-doc sets.
// Run executes via the platform query API — edit the JSON filter below.
{ "status": "active" }`,
    supportsRaw: false,
    rawNote: "Raw Redis commands aren't executed server-side yet — use the builder.",
  },
  arcadedb: {
    kind: "arcade",
    languageLabel: "ArcadeDB SQL",
    placeholder: '{ "name": "Alice" }',
    sample: `// ArcadeDB speaks SQL over HTTP (e.g. SELECT * FROM Person LIMIT 25).
// Run executes via the platform query API — edit the JSON filter below.
{}`,
    supportsRaw: false,
    rawNote: "Raw ArcadeDB SQL runs through the query builder for now.",
  },
  qdrant: {
    kind: "vector",
    languageLabel: "Qdrant (payload filter)",
    placeholder: '{ "status": "active" }',
    sample: `// Qdrant: points surface as rows; payload fields are columns.
// Run executes via the platform query API with this payload filter.
{ "status": "active" }`,
    supportsRaw: false,
    rawNote: "Vector similarity search is roadmap — payload filtering works today.",
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
