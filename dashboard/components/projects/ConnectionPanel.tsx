"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { TableBrowser } from "@/components/data/TableBrowser";
import { Badge, Button, EmptyState, ErrorBanner, Input, Label, Spinner } from "@/components/ui";
import type { Connection } from "@/lib/types";

export function ConnectionPanel({ projectId }: { projectId: string }) {
  const [conn, setConn] = useState<Connection | null | "loading">("loading");
  const [connString, setConnString] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      setConn(await api.getConnection(token, projectId));
    } catch (err) {
      const e = err as { status?: number };
      setConn(e.status === 404 ? null : (e as any));
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!connString.trim()) return;
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.saveConnection(token, projectId, connString);
      if (res.success) {
        setMessage(`Connected! Detected engine: ${res.engine ?? "unknown"}`);
        setConnString("");
        await load();
      } else {
        setError(res.message ?? "Connection failed");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save connection");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6">
      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Connect your database</h2>
        <form onSubmit={save} className="max-w-xl space-y-3 rounded-xl border border-slate-200 bg-white p-4">
          <div>
            <Label htmlFor="conn-string">Connection string</Label>
            <Input
              id="conn-string"
              type="password"
              autoComplete="off"
              value={connString}
              onChange={(e) => setConnString(e.target.value)}
              placeholder="postgres://user:pass@host:5432/db"
            />
            <p className="mt-1 text-xs text-slate-400">
              The engine is auto-detected from the URL scheme. Credentials are
              encrypted at rest (SCHEMA.md §2). Postgres and FerretDB supported so far.
            </p>
          </div>
          {error && <ErrorBanner message={error} />}
          {message && (
            <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-700">
              {message}
            </div>
          )}
          <Button type="submit" loading={saving}>
            Save connection
          </Button>
        </form>
      </section>

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Current connection</h2>
        {conn === "loading" ? (
          <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        ) : conn && !(conn as any).status ? (
          <p className="text-sm text-slate-500">No connection configured for this project.</p>
        ) : conn ? (
          <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-4">
            <Badge tone="blue">{(conn as Connection).engine}</Badge>
            <Badge tone="amber">
              {(conn as Connection).mode === "provisioned" ? "Provisioned" : "BYODB"}
            </Badge>
            <Badge tone={(conn as Connection).status === "connected" ? "green" : "amber"}>
              {(conn as Connection).status}
            </Badge>
            {(conn as Connection).last_checked_at && (
              <span className="text-xs text-slate-500">
                checked {new Date((conn as Connection).last_checked_at!).toLocaleString()}
              </span>
            )}
          </div>
        ) : (
          <EmptyState
            title="No database connected"
            hint="Paste a connection string above to attach a database to this project."
          />
        )}
      </section>

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Data browser</h2>
        {conn === "loading" ? (
          <Spinner className="h-4 w-4" />
        ) : conn && (conn as Connection).status === "connected" ? (
          <TableBrowser projectId={projectId} />
        ) : (
          <EmptyState
            title="Data browser waits for a connection"
            hint="Once connected, browse tables and rows here — like Supabase's table editor."
          />
        )}
      </section>
    </div>
  );
}