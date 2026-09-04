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
  -d '{"name":"Ada"}'`;
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
