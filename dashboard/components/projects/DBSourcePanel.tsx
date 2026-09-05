"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { TableBrowser } from "@/components/data/TableBrowser";
import { Button, EmptyState, ErrorBanner, Input, Label } from "@/components/ui";
import { Badge, StatusBadge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { TableBrowserSkeleton } from "@/components/ui/skeletons";
import { SegmentedOption } from "@/components/ui/segmented";
import type { Connection } from "@/lib/types";

type ConnectMode = "byodb" | "provisioned";
type EngineChoice = "postgres" | "ferretdb";

/**
 * DB Source tab: attaches the project's *backing* database (inbound —
 * platform → your DB). The outbound direction (your app → Openbase) lives in
 * the Connect tab.
 */
export function DBSourcePanel({ projectId }: { projectId: string }) {
  const [conn, setConn] = useState<Connection | null | "loading">("loading");
  const [mode, setMode] = useState<ConnectMode>("byodb");
  const [engine, setEngine] = useState<EngineChoice>("postgres");
  const [connString, setConnString] = useState("");
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
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

  const isAutoProvision = mode === "byodb" && !connString.trim();

  async function save(e: React.FormEvent) {
    e.preventDefault();
    // Empty connection string => auto-provision a Postgres instance (backend
    // fallback when both mode and connection_string are empty). We send an
    // explicit provisioned request so the intent is clear even if the backend
    // fallback ever changes.
    const effectiveMode = isAutoProvision ? "provisioned" : mode;
    const effectiveEngine = effectiveMode === "provisioned" ? (mode === "provisioned" ? engine : "postgres") : undefined;
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.saveConnection(
        token,
        projectId,
        connString.trim(),
        effectiveMode,
        effectiveEngine
      );
      if (res.success) {
        setMessage(
          effectiveMode === "provisioned"
            ? isAutoProvision
              ? `No connection string provided — provisioned a Postgres database for this project.`
              : `Provisioned a ${effectiveEngine ?? engine} database for this project.`
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
    setRemoving(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteConnection(token, projectId);
      setConn(null);
      setConfirmRemove(false);
      setMessage("Connection removed.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove connection");
    } finally {
      setRemoving(false);
    }
  }

  const connected = conn !== "loading" && conn !== null && (conn as Connection)?.status === "connected";
  const modeLabel = (conn as Connection)?.mode === "provisioned" ? "Provisioned" : "BYODB";
  const isInitialLoading = conn === "loading";

  return (
    <div className="space-y-6">
      <p className="max-w-xl text-label text-muted-foreground">
        This tab points Openbase <strong className="font-w510 text-foreground-strong">at</strong> a
        database. To point an app at Openbase, use the Connect tab.
      </p>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">
          {isInitialLoading ? "Loading DB source…" : connected ? "Replace database" : "Set up a database"}
        </h2>

        <div className="mb-3 flex max-w-xl flex-wrap gap-2">
          <SegmentedOption selected={mode === "provisioned"} onClick={() => setMode("provisioned")}>
            New provisioned DB
          </SegmentedOption>
          <SegmentedOption selected={mode === "byodb"} onClick={() => setMode("byodb")}>
            Connect existing
          </SegmentedOption>
        </div>

        <form onSubmit={save} className="max-w-xl space-y-3 rounded-lg border border-border bg-card p-4">
          {mode === "provisioned" ? (
            <div className="space-y-3">
              <div>
                <Label htmlFor="engine">Engine</Label>
                <div className="mt-1 flex flex-wrap gap-2">
                  {(["postgres", "ferretdb"] as EngineChoice[]).map((e) => (
                    <SegmentedOption key={e} selected={engine === e} onClick={() => setEngine(e)}>
                      {e === "postgres" ? "PostgreSQL" : "FerretDB (Mongo-compatible)"}
                    </SegmentedOption>
                  ))}
                </div>
              </div>
              <div>
                <p className="text-caption text-muted-foreground">
                  Openbase spins up a dedicated {engine === "postgres" ? "Postgres" : "FerretDB"} container,
                  generates its credentials, and encrypts them at rest (SCHEMA.md §2).
                </p>
                <p className="mt-1.5 text-label text-muted-foreground">
                  {engine === "postgres"
                    ? "Full relational capabilities: tables, joins, constraints."
                    : "Mongo wire-compatible document store via FerretDB on Postgres; no relations yet (honest Capabilities)."}
                </p>
              </div>
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
                placeholder="postgres://user:pass@host:5432/db, mysql://user:pass@host:3306/db, mongodb://user:pass@host:27017/, redis://host:6379, http://host:6333 (Qdrant), or http://host:2480 (ArcadeDB)"
                />
              <p className="mt-1.5 text-label text-muted-foreground">
                The engine is auto-detected from the URL scheme: postgres://, mysql://,
                mongodb://, redis:// (Valkey), http(s)://…:6333 (Qdrant), or http(s)://…:2480
                (ArcadeDB). Credentials are
                encrypted at rest (SCHEMA.md §2). Leave empty to auto-provision a
                Postgres database instead.
              </p>
            </div>
          )}
          {error && <ErrorBanner message={error} />}
          {message && (
            <div className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-caption text-success">
              {message}
            </div>
          )}
          <Button type="submit" loading={busy}>
            {mode === "provisioned"
              ? "Provision database"
              : isAutoProvision
                ? "Auto-provision Postgres"
                : "Save connection"}
          </Button>
        </form>
      </section>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Current DB source</h2>
        {conn === "loading" ? (
          <div role="status" aria-label="Loading current DB source" className="max-w-xl rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-3">
              <Skeleton className="h-5 w-20" />
              <Skeleton className="h-5 w-24" />
              <Skeleton className="h-5 w-20" />
            </div>
            <span className="sr-only">Loading current DB source…</span>
          </div>
        ) : connected ? (
          <div className="flex max-w-xl items-center justify-between gap-3 rounded-lg border border-border bg-card p-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary">{(conn as Connection).engine}</Badge>
              <Badge variant="secondary">{modeLabel}</Badge>
              <StatusBadge tone="success">connected</StatusBadge>
              {(conn as Connection).last_checked_at && (
                <span className="font-mono text-label text-muted-foreground">
                  checked {new Date((conn as Connection).last_checked_at!).toLocaleString()}
                </span>
              )}
            </div>
            <Button variant="danger" onClick={() => setConfirmRemove(true)} loading={removing}>
              Remove
            </Button>
          </div>
        ) : (
          <EmptyState
            title="No database attached"
            hint="Provision a new one or attach an existing database above."
          />
        )}
      </section>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Data browser</h2>
        {conn === "loading" ? (
          <TableBrowserSkeleton label="Loading data browser" />
        ) : connected ? (
          <TableBrowser projectId={projectId} />
        ) : (
          <EmptyState
            title="Data browser waits for a connection"
            hint="Once connected, browse tables and rows here — like Supabase's table editor."
          />
        )}
      </section>

      <ConfirmDialog
        open={confirmRemove}
        onOpenChange={setConfirmRemove}
        title="Remove this database?"
        description={
          (conn as Connection)?.mode === "provisioned"
            ? "The provisioned container and all data in it are destroyed. This is not recoverable — there is no backup."
            : "Openbase forgets the saved credentials. Your database itself is left untouched."
        }
        confirmLabel="Remove database"
        loading={removing}
        onConfirm={remove}
      />
    </div>
  );
}