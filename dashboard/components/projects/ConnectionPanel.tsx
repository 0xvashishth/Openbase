"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { TableBrowser } from "@/components/data/TableBrowser";
import { Badge, Button, EmptyState, ErrorBanner, Input, Label, Spinner } from "@/components/ui";
import type { Connection } from "@/lib/types";

type ConnectMode = "byodb" | "provisioned";

export function ConnectionPanel({ projectId }: { projectId: string }) {
  const [conn, setConn] = useState<Connection | null | "loading">("loading");
  const [mode, setMode] = useState<ConnectMode>("byodb");
  const [connString, setConnString] = useState("");
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState(false);
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
    if (mode === "byodb" && !connString.trim()) return;
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.saveConnection(token, projectId, connString.trim(), mode);
      if (res.success) {
        setMessage(
          mode === "provisioned"
            ? `Provisioned a Postgres database for this project.`
            : `Connected! Detected engine: ${res.engine ?? "unknown"}`
        );
        setConnString("");
        await load();
      } else {
        setError(res.message ?? "Connection failed");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save connection");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm("Remove this connection? A provisioned database will be destroyed.")) return;
    setRemoving(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteConnection(token, projectId);
      setConn(null);
      setMessage("Connection removed.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove connection");
    } finally {
      setRemoving(false);
    }
  }

  const connected = conn !== "loading" && conn !== null && (conn as Connection)?.status === "connected";
  const modeLabel = (conn as Connection)?.mode === "provisioned" ? "Provisioned" : "BYODB";

  return (
    <div className="space-y-6">
      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">
          {connected ? "Replace database" : "Set up a database"}
        </h2>

        <div className="mb-3 flex max-w-xl gap-2">
          <button
            type="button"
            onClick={() => setMode("provisioned")}
            className={`rounded-lg border px-3 py-1.5 text-sm font-medium ${
              mode === "provisioned"
                ? "border-brand-600 bg-brand-50 text-brand-700"
                : "border-slate-200 text-slate-600 hover:bg-slate-50"
            }`}
          >
            New provisioned DB
          </button>
          <button
            type="button"
            onClick={() => setMode("byodb")}
            className={`rounded-lg border px-3 py-1.5 text-sm font-medium ${
              mode === "byodb"
                ? "border-brand-600 bg-brand-50 text-brand-700"
                : "border-slate-200 text-slate-600 hover:bg-slate-50"
            }`}
          >
            Connect existing
          </button>
        </div>

        <form onSubmit={save} className="max-w-xl space-y-3 rounded-xl border border-slate-200 bg-white p-4">
          {mode === "provisioned" ? (
            <div>
              <Label>Engine</Label>
              <p className="text-sm text-slate-700">
                Postgres <span className="text-xs text-slate-500">(more engines arrive with Phase 2)</span>
              </p>
              <p className="mt-1 text-xs text-slate-400">
                Openbase spins up a dedicated Postgres container, generates its
                credentials, and encrypts them at rest (SCHEMA.md §2).
              </p>
            </div>
          ) : (
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
                encrypted at rest (SCHEMA.md §2).
              </p>
            </div>
          )}
          {error && <ErrorBanner message={error} />}
          {message && (
            <div className="rounded-md border border-green-200 bg-green-50 px-3 py-2 text-sm text-green-700">
              {message}
            </div>
          )}
          <Button type="submit" loading={busy}>
            {mode === "provisioned" ? "Provision database" : "Save connection"}
          </Button>
        </form>
      </section>

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Current connection</h2>
        {conn === "loading" ? (
          <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        ) : connected ? (
          <div className="flex max-w-xl items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-center gap-3">
              <Badge tone="blue">{(conn as Connection).engine}</Badge>
              <Badge tone="amber">{modeLabel}</Badge>
              <Badge tone="green">connected</Badge>
              {(conn as Connection).last_checked_at && (
                <span className="text-xs text-slate-500">
                  checked {new Date((conn as Connection).last_checked_at!).toLocaleString()}
                </span>
              )}
            </div>
            <Button variant="danger" onClick={remove} loading={removing}>
              Remove
            </Button>
          </div>
        ) : (
          <EmptyState
            title="No database connected"
            hint="Provision a new one or attach an existing database above."
          />
        )}
      </section>

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Data browser</h2>
        {conn === "loading" ? (
          <Spinner className="h-4 w-4" />
        ) : connected ? (
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