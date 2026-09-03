"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, API_URL } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { openbaseRealtime, type LiveClient } from "@/lib/realtime";
import type { RealtimeChange } from "@/lib/realtime";
import { Button, ErrorBanner, Input, Label } from "@/components/ui";
import { ToolPageSkeleton } from "@/components/ui/skeletons";

interface Row {
  [key: string]: unknown;
}

export function RealtimeDemo({ projectId }: { projectId: string }) {
  const [collections, setCollections] = useState<string[]>([]);
  const [collection, setCollection] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [inserting, setInserting] = useState(false);
  const [insertName, setInsertName] = useState("");

  const clientRef = useRef<LiveClient | null>(null);

  // Hold the plaintext API key in memory (never persisted to state renders).
  const keyRef = useRef<string | null>(null);

  const loadCollections = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    try {
      const cols = await api.listCollections(token, projectId);
      setCollections(cols.map((c) => c.name));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load collections");
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  // Ensure we have an API key on mount (the realtime WS + REST both need one).
  useEffect(() => {
    async function ensureKey() {
      const token = authToken();
      if (!token) return;
      try {
        const keys = await api.listAPIKeys(token, projectId);
        if (keys.length > 0) return; // a key already exists
        const created = await api.createAPIKey(token, projectId, "realtime-demo");
        keyRef.current = created.plaintext;
      } catch {
        // ignore
      }
    }
    ensureKey();
    loadCollections();
  }, [projectId, loadCollections]);

  async function start() {
    if (!collection.trim()) return;
    const token = authToken();
    if (!token) return;
    setError(null);

    // Resolve an API key, creating one if needed.
    let key = keyRef.current;
    if (!key) {
      try {
        const keys = await api.listAPIKeys(token, projectId);
        key = keys.length ? null : (await api.createAPIKey(token, projectId, "realtime-demo")).plaintext;
        keyRef.current = key ?? null;
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to get API key");
        return;
      }
    }
    if (!key) {
      // An existing key exists but we don't have its plaintext; tell the user
      // to create a fresh key so the demo can authenticate the socket.
      setError("Create a new API key on the API Keys tab, then retry.");
      return;
    }

    // Load initial rows.
    try {
      const rs = await api.queryRows(token, projectId, { collection, limit: 20 });
      setRows(rs.rows);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load rows");
      return;
    }

    // Subscribe to live changes.
    const wsBase = API_URL.replace(/^http/, "ws");
    const client = openbaseRealtime(`${wsBase}/v1/realtime`, key);
    clientRef.current = client;
    setConnected(true);

    client.subscribe<Row>(
      collection,
      (change: RealtimeChange<Row>) => handleChange(change),
      (msg) => {
        if (msg.error) setError(msg.error);
      }
    );
  }

  function handleChange(change: RealtimeChange<Row>) {
    const row = change.data;
    const id = String(row.id);
    setRows((prev) => {
      const next = prev.filter((r) => String(r.id) !== id);
      next.push(row);
      return next.slice(-50);
    });
  }

  async function insertRow(e: React.FormEvent) {
    e.preventDefault();
    if (!insertName.trim() || !collection.trim()) return;
    const token = authToken();
    const key = keyRef.current;
    if (!token || !key) return;
    setInserting(true);
    setError(null);
    try {
      const res = await fetch(`${API_URL}/v1/api/${collection}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${key}`,
        },
        body: JSON.stringify({ name: insertName.trim() }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error ?? "insert failed");
      }
      setInsertName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Insert failed");
    } finally {
      setInserting(false);
    }
  }

  useEffect(() => {
    return () => {
      if (clientRef.current) clientRef.current.close();
    };
  }, []);

  if (loading) {
    return <ToolPageSkeleton label="Loading collections" />;
  }

  return (
    <div className="space-y-4">
      <section className="max-w-2xl">
        <h2 className="mb-2 text-sm font-semibold text-foreground">Live updates</h2>
        <p className="mb-3 text-xs text-muted-foreground">
          Pick a table and subscribe. Insert a row (via the auto-generated REST API) and watch it
          appear instantly over the WebSocket gateway.
        </p>
        <div className="flex items-end gap-3">
          <div className="flex-1">
            <Label htmlFor="rt-collection">Collection</Label>
            <select
              id="rt-collection"
              value={collection}
              onChange={(e) => setCollection(e.target.value)}
              className="w-full rounded-lg border border-input bg-card px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">Select a table…</option>
              {collections.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
          <Button onClick={start} disabled={!collection.trim()}>
            Subscribe
          </Button>
        </div>
        {connected && (
          <div className="mt-2 flex items-center gap-2 text-xs">
            <span className="h-2 w-2 rounded-full bg-success/100" />
            <span className="text-muted-foreground">live — listening on {collection}</span>
          </div>
        )}
      </section>

      {error && <ErrorBanner message={error} />}

      {connected && (
        <section className="max-w-2xl">
          <h3 className="mb-2 text-sm font-semibold text-foreground">Insert a row</h3>
          <form onSubmit={insertRow} className="flex items-end gap-3">
            <div className="flex-1">
              <Label htmlFor="rt-name">name</Label>
              <Input
                id="rt-name"
                value={insertName}
                onChange={(e) => setInsertName(e.target.value)}
                placeholder="e.g. Mint Tea"
              />
            </div>
            <Button type="submit" loading={inserting}>
              Insert
            </Button>
          </form>
        </section>
      )}

      {connected && (
        <section className="max-w-2xl">
          <h3 className="mb-2 text-sm font-semibold text-foreground">
            {collection} <span className="font-normal text-muted-foreground">(live)</span>
          </h3>
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            {rows.length === 0 ? (
              <div className="px-4 py-6 text-center text-sm text-muted-foreground">No rows yet.</div>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-muted text-xs uppercase text-muted-foreground">
                  <tr>
                    {Object.keys(rows[0]).map((k) => (
                      <th key={k} className="px-4 py-2 font-medium">
                        {k}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {rows.map((r, i) => (
                    <tr key={i} className="text-muted-foreground">
                      {Object.entries(r).map(([k, v]) => (
                        <td key={k} className="px-4 py-2">
                          {String(v)}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
