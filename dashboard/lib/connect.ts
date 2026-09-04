// Helpers for the project "Connect" tab: everything an external app needs to
// talk TO Openbase (outbound direction). The DB Source tab covers the inbound
// direction (Openbase → your database).
//
// Pure functions only — no React, no window access — so they are unit-testable
// and safe to call during SSR. Callers pass the environment in.

/** Shown in snippets until the user picks/creates a real key. */
export const API_KEY_PLACEHOLDER = "ob_your_api_key";

/** Shown in snippets until a real collection is known. */
export const TABLE_PLACEHOLDER = "your_table";

export interface ConnectParams {
  /** Public base URL of the Openbase API, no trailing slash. */
  apiBaseUrl: string;
  /** Plaintext API key, or API_KEY_PLACEHOLDER. */
  apiKey: string;
  /** Table/collection used in the examples. */
  table: string;
  /**
   * Realtime endpoint. Defaults to the ws(s) form of apiBaseUrl; pass it
   * explicitly when the server advertises a different one via /connect-info.
   */
  wsUrl?: string;
}

/** Strip trailing slashes so `${base}/v1/...` never doubles up. */
export function normalizeBaseUrl(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/**
 * Resolve the API base URL to advertise in snippets.
 *
 * The dashboard talks to the API same-origin (Next rewrites `/v1/*`), so
 * `window.location.origin` is the correct default for copy-paste. Deployments
 * that expose the API on its own hostname set NEXT_PUBLIC_OPENBASE_API_URL.
 */
export function resolveApiBaseUrl(opts?: { envUrl?: string; origin?: string }): string {
  const env = normalizeBaseUrl(opts?.envUrl ?? "");
  if (env) return env;
  const origin = normalizeBaseUrl(opts?.origin ?? "");
  if (origin) return origin;
  return "http://localhost:8080";
}

/** WebSocket URL for the realtime gateway (http→ws, https→wss). */
export function realtimeUrl(apiBaseUrl: string): string {
  return `${normalizeBaseUrl(apiBaseUrl).replace(/^http/, "ws")}/v1/realtime`;
}

/** REST URL for a collection, e.g. https://host/v1/api/users */
export function restUrl(apiBaseUrl: string, table: string): string {
  return `${normalizeBaseUrl(apiBaseUrl)}/v1/api/${table}`;
}

export function curlSnippet({ apiBaseUrl, apiKey, table }: ConnectParams): string {
  const base = normalizeBaseUrl(apiBaseUrl);
  return `# List tables
curl "${base}/v1/api/tables" \\
  -H "Authorization: Bearer ${apiKey}"

# Read rows
curl "${base}/v1/api/${table}?limit=10&order_by=id&order=desc" \\
  -H "Authorization: Bearer ${apiKey}"

# Insert a row
curl -X POST "${base}/v1/api/${table}" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -H "Content-Type: application/json" \\
  -d '{"name":"Ada"}'

# Update / delete by id
curl -X PUT "${base}/v1/api/${table}/1" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -H "Content-Type: application/json" \\
  -d '{"name":"Grace"}'

curl -X DELETE "${base}/v1/api/${table}/1" \\
  -H "Authorization: Bearer ${apiKey}"`;
}

export function jsSnippet({ apiBaseUrl, apiKey, table }: ConnectParams): string {
  const base = normalizeBaseUrl(apiBaseUrl);
  return `const OPENBASE_URL = "${base}";
const OPENBASE_API_KEY = process.env.OPENBASE_API_KEY; // ${apiKey}

async function openbase(path, init = {}) {
  const res = await fetch(\`\${OPENBASE_URL}/v1/api\${path}\`, {
    ...init,
    headers: {
      Authorization: \`Bearer \${OPENBASE_API_KEY}\`,
      "Content-Type": "application/json",
      ...init.headers,
    },
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

// Read rows
const rows = await openbase("/${table}?limit=10");

// Insert a row
await openbase("/${table}", {
  method: "POST",
  body: JSON.stringify({ name: "Ada" }),
});`;
}

/**
 * Typed client for TS projects. Written as a module you can drop in
 * (lib/openbase.ts) plus a Next.js server-side usage example — keys must stay
 * server-side, so it reads a non-public env var.
 */
export function tsSnippet({ apiBaseUrl, apiKey, table }: ConnectParams): string {
  const base = normalizeBaseUrl(apiBaseUrl);
  return `// lib/openbase.ts
const OPENBASE_URL = process.env.OPENBASE_URL ?? "${base}";
const OPENBASE_API_KEY = process.env.OPENBASE_API_KEY!; // ${apiKey}

export async function openbase<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(\`\${OPENBASE_URL}/v1/api\${path}\`, {
    ...init,
    headers: {
      Authorization: \`Bearer \${OPENBASE_API_KEY}\`,
      "Content-Type": "application/json",
      ...init.headers,
    },
    cache: "no-store",
  });
  if (!res.ok) throw new Error(\`openbase \${res.status}: \${await res.text()}\`);
  return res.json() as Promise<T>;
}

// app/page.tsx — server component keeps the key off the client
interface ${pascalCase(table)} { id: number; name: string }

export default async function Page() {
  const rows = await openbase<${pascalCase(table)}[]>("/${table}?limit=10");
  return <ul>{rows.map((r) => <li key={r.id}>{r.name}</li>)}</ul>;
}`;
}

export function pythonSnippet({ apiBaseUrl, apiKey, table }: ConnectParams): string {
  const base = normalizeBaseUrl(apiBaseUrl);
  return `import os
import httpx

OPENBASE_URL = os.environ.get("OPENBASE_URL", "${base}")
OPENBASE_API_KEY = os.environ["OPENBASE_API_KEY"]  # ${apiKey}

client = httpx.Client(
    base_url=f"{OPENBASE_URL}/v1/api",
    headers={"Authorization": f"Bearer {OPENBASE_API_KEY}"},
)

# Read rows
rows = client.get("/${table}", params={"limit": 10, "order_by": "id"}).raise_for_status().json()

# Insert a row
client.post("/${table}", json={"name": "Ada"}).raise_for_status()`;
}

export function goSnippet({ apiBaseUrl, apiKey, table }: ConnectParams): string {
  const base = normalizeBaseUrl(apiBaseUrl);
  return `package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func main() {
	baseURL := os.Getenv("OPENBASE_URL") // ${base}
	apiKey := os.Getenv("OPENBASE_API_KEY") // ${apiKey}

	req, err := http.NewRequest("GET", baseURL+"/v1/api/${table}?limit=10", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var rows []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		panic(err)
	}
	fmt.Println(len(rows), "rows")
}`;
}

export function realtimeSnippet({ apiBaseUrl, apiKey, table, wsUrl }: ConnectParams): string {
  return `// Browser / Node 22+ (native WebSocket)
const socket = new WebSocket("${wsUrl ?? realtimeUrl(apiBaseUrl)}", {
  headers: { Authorization: "Bearer ${apiKey}" }, // Node: use the ws package
});

socket.onopen = () => {
  socket.send(JSON.stringify({ type: "subscribe", collection: "${table}" }));
};

socket.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "change") {
    console.log(msg.event, msg.collection, msg.data); // insert | update | delete
  }
};`;
}

export function envSnippet({ apiBaseUrl, apiKey }: Omit<ConnectParams, "table">): string {
  return `OPENBASE_URL=${normalizeBaseUrl(apiBaseUrl)}
OPENBASE_API_KEY=${apiKey}`;
}

export interface SnippetDef {
  id: string;
  /** Tab label. */
  label: string;
  /** Language tag shown on the code block. */
  language: string;
  build: (params: ConnectParams) => string;
}

/**
 * Client snippets in the order shown by the Connect tab. Registry-driven so
 * adding a language means one entry here, and tests can assert invariants
 * (base URL present, key present, no DB credentials) across all of them.
 */
export const CLIENT_SNIPPETS: SnippetDef[] = [
  { id: "curl", label: "cURL", language: "bash", build: curlSnippet },
  { id: "javascript", label: "JavaScript", language: "javascript", build: jsSnippet },
  { id: "typescript", label: "TypeScript / Next.js", language: "tsx", build: tsSnippet },
  { id: "python", label: "Python", language: "python", build: pythonSnippet },
  { id: "go", label: "Go", language: "go", build: goSnippet },
];

/** Title-cases a table name for use as a type name, e.g. order_items → OrderItems. */
export function pascalCase(name: string): string {
  const parts = name.split(/[^A-Za-z0-9]+/).filter(Boolean);
  if (parts.length === 0) return "Row";
  return parts.map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join("");
}

/**
 * Guidance for the "Direct database" section. Openbase never hands out the
 * project's database credentials: provisioned instances keep them encrypted
 * server-side, and BYODB strings belong to the user's own provider. ORMs
 * therefore target the source database, not Openbase.
 */
export function directDbGuidance(mode?: string): { title: string; body: string } {
  if (mode === "provisioned") {
    return {
      title: "Provisioned database",
      body:
        "Openbase generated and encrypted these credentials, and does not expose them. " +
        "Use the REST API above, or replace this with a bring-your-own database on the " +
        "DB Source tab if you need direct ORM access.",
    };
  }
  if (mode === "byodb") {
    return {
      title: "Bring-your-own database",
      body:
        "You already hold this connection string — point Prisma, Drizzle, SQLAlchemy or " +
        "any ORM straight at your provider. Openbase stores it encrypted and never returns it.",
    };
  }
  return {
    title: "No database attached",
    body: "Attach a database on the DB Source tab to see how ORMs fit in.",
  };
}
