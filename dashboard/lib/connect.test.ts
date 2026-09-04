import { describe, expect, it } from "vitest";
import {
  API_KEY_PLACEHOLDER,
  CLIENT_SNIPPETS,
  TABLE_PLACEHOLDER,
  curlSnippet,
  directDbGuidance,
  envSnippet,
  goSnippet,
  jsSnippet,
  normalizeBaseUrl,
  pascalCase,
  pythonSnippet,
  realtimeSnippet,
  realtimeUrl,
  resolveApiBaseUrl,
  restUrl,
  tsSnippet,
} from "./connect";

const params = { apiBaseUrl: "https://api.example.com", apiKey: "ob_test", table: "orders" };

describe("normalizeBaseUrl", () => {
  it("trims whitespace and trailing slashes", () => {
    expect(normalizeBaseUrl("  https://api.example.com/  ")).toBe("https://api.example.com");
    expect(normalizeBaseUrl("https://api.example.com///")).toBe("https://api.example.com");
  });
});

describe("resolveApiBaseUrl", () => {
  it("prefers the configured public URL", () => {
    expect(
      resolveApiBaseUrl({ envUrl: "https://api.example.com/", origin: "https://dash.example.com" })
    ).toBe("https://api.example.com");
  });

  it("falls back to the dashboard origin (same-origin rewrites)", () => {
    expect(resolveApiBaseUrl({ envUrl: "", origin: "https://dash.example.com" })).toBe(
      "https://dash.example.com"
    );
  });

  it("falls back to the local dev API when nothing is known", () => {
    expect(resolveApiBaseUrl()).toBe("http://localhost:8080");
    expect(resolveApiBaseUrl({ envUrl: "  ", origin: "" })).toBe("http://localhost:8080");
  });
});

describe("realtimeUrl", () => {
  it("maps https to wss and http to ws", () => {
    expect(realtimeUrl("https://api.example.com")).toBe("wss://api.example.com/v1/realtime");
    expect(realtimeUrl("http://localhost:8080")).toBe("ws://localhost:8080/v1/realtime");
  });

  it("never doubles slashes", () => {
    expect(realtimeUrl("https://api.example.com/")).toBe("wss://api.example.com/v1/realtime");
  });
});

describe("restUrl", () => {
  it("builds the auto-REST collection URL", () => {
    expect(restUrl("https://api.example.com/", "users")).toBe("https://api.example.com/v1/api/users");
  });
});

describe("snippets", () => {
  it("curl covers list/read/insert with the bearer key", () => {
    const out = curlSnippet(params);
    expect(out).toContain("https://api.example.com/v1/api/tables");
    expect(out).toContain("https://api.example.com/v1/api/orders?limit=10");
    expect(out).toContain('-X POST "https://api.example.com/v1/api/orders"');
    expect(out).toContain("Authorization: Bearer ob_test");
  });

  it("javascript snippet wires the base URL and key from env", () => {
    const out = jsSnippet(params);
    expect(out).toContain('const OPENBASE_URL = "https://api.example.com"');
    expect(out).toContain("process.env.OPENBASE_API_KEY");
    expect(out).toContain('openbase("/orders?limit=10")');
  });

  it("realtime snippet subscribes over the ws gateway", () => {
    const out = realtimeSnippet(params);
    expect(out).toContain("wss://api.example.com/v1/realtime");
    expect(out).toContain('{ type: "subscribe", collection: "orders" }');
    expect(out).toContain("Bearer ob_test");
  });

  it("realtime snippet honours a server-advertised ws URL", () => {
    const out = realtimeSnippet({ ...params, wsUrl: "wss://edge.example.com/v1/realtime" });
    expect(out).toContain("wss://edge.example.com/v1/realtime");
    expect(out).not.toContain("wss://api.example.com/v1/realtime");
  });

  it("env snippet exposes only URL + key", () => {
    expect(envSnippet({ apiBaseUrl: "https://api.example.com/", apiKey: "ob_test" })).toBe(
      "OPENBASE_URL=https://api.example.com\nOPENBASE_API_KEY=ob_test"
    );
  });

  it("uses placeholders when no key/table is known yet", () => {
    const out = curlSnippet({
      apiBaseUrl: "http://localhost:8080",
      apiKey: API_KEY_PLACEHOLDER,
      table: TABLE_PLACEHOLDER,
    });
    expect(out).toContain("ob_your_api_key");
    expect(out).toContain("your_table");
  });

  it("never leaks a real connection string into snippets", () => {
    for (const snippet of [curlSnippet(params), jsSnippet(params), realtimeSnippet(params)]) {
      expect(snippet).not.toMatch(/postgres:\/\/|mysql:\/\/|mongodb:\/\//);
    }
  });
});

describe("language snippets", () => {
  it("typescript snippet keeps the key server-side and types the rows", () => {
    const out = tsSnippet({ ...params, table: "order_items" });
    expect(out).toContain("process.env.OPENBASE_API_KEY!");
    expect(out).not.toContain("NEXT_PUBLIC_OPENBASE_API_KEY");
    expect(out).toContain("interface OrderItems");
    expect(out).toContain('openbase<OrderItems[]>("/order_items?limit=10")');
  });

  it("python snippet uses httpx with a bearer header", () => {
    const out = pythonSnippet(params);
    expect(out).toContain("import httpx");
    expect(out).toContain('f"{OPENBASE_URL}/v1/api"');
    expect(out).toContain('Bearer {OPENBASE_API_KEY}');
    expect(out).toContain('client.post("/orders"');
  });

  it("go snippet reads config from the environment", () => {
    const out = goSnippet(params);
    expect(out).toContain('os.Getenv("OPENBASE_API_KEY")');
    expect(out).toContain('baseURL+"/v1/api/orders?limit=10"');
    expect(out).toContain('req.Header.Set("Authorization", "Bearer "+apiKey)');
  });

  it("curl snippet covers update and delete too", () => {
    const out = curlSnippet(params);
    expect(out).toContain('-X PUT "https://api.example.com/v1/api/orders/1"');
    expect(out).toContain('-X DELETE "https://api.example.com/v1/api/orders/1"');
  });
});

describe("CLIENT_SNIPPETS registry", () => {
  it("exposes stable ids and labels", () => {
    expect(CLIENT_SNIPPETS.map((s) => s.id)).toEqual([
      "curl",
      "javascript",
      "typescript",
      "python",
      "go",
    ]);
    for (const s of CLIENT_SNIPPETS) {
      expect(s.label).toBeTruthy();
      expect(s.language).toBeTruthy();
    }
  });

  it("every snippet carries the base URL, the key and the table", () => {
    for (const s of CLIENT_SNIPPETS) {
      const out = s.build(params);
      expect(out, s.id).toContain("https://api.example.com");
      expect(out, s.id).toContain("ob_test");
      expect(out, s.id).toContain("orders");
    }
  });

  it("no snippet embeds database credentials", () => {
    for (const s of CLIENT_SNIPPETS) {
      expect(s.build(params), s.id).not.toMatch(/postgres:\/\/|mysql:\/\/|mongodb:\/\/|redis:\/\//);
    }
  });
});

describe("pascalCase", () => {
  it("builds type names from table names", () => {
    expect(pascalCase("orders")).toBe("Orders");
    expect(pascalCase("order_items")).toBe("OrderItems");
    expect(pascalCase("public.order-items")).toBe("PublicOrderItems");
    expect(pascalCase("")).toBe("Row");
  });
});

describe("directDbGuidance", () => {
  it("explains that provisioned credentials are never exposed", () => {
    const g = directDbGuidance("provisioned");
    expect(g.title).toMatch(/provisioned/i);
    expect(g.body).toMatch(/does not expose them/i);
  });

  it("points BYODB users at their own provider string", () => {
    const g = directDbGuidance("byodb");
    expect(g.title).toMatch(/bring-your-own/i);
    expect(g.body).toMatch(/never returns it/i);
  });

  it("falls back when no database is attached", () => {
    expect(directDbGuidance(undefined).title).toMatch(/no database/i);
  });
});
