"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CodeBlock, CopyField } from "@/components/ui/code-block";
import { EmptyState, ErrorBanner } from "@/components/ui/feedback";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FormSkeleton, ListSkeleton } from "@/components/ui/skeletons";
import { useProject } from "@/lib/project-context";
import {
  API_KEY_PLACEHOLDER,
  CLIENT_SNIPPETS,
  TABLE_PLACEHOLDER,
  directDbGuidance,
  envSnippet,
  realtimeSnippet,
  realtimeUrl,
  resolveApiBaseUrl,
} from "@/lib/connect";
import type { APIKeyView, ConnectInfo } from "@/lib/types";

/**
 * Connect tab: how an external app talks TO Openbase (outbound). The DB Source
 * tab is the opposite direction — attaching the project's backing database.
 *
 * Keys are hash-only at rest, so a freshly created key is the only time we can
 * show a real token. Existing keys fall back to a placeholder in snippets.
 */
export function ConnectPanel({ projectId }: { projectId: string }) {
  const { orgId, engine, hasConnection, supportsRealtime } = useProject();
  const [info, setInfo] = useState<ConnectInfo | null>(null);
  const [keys, setKeys] = useState<APIKeyView[] | null>(null);
  const [tables, setTables] = useState<string[]>([]);
  const [table, setTable] = useState<string>("");
  const [freshKey, setFreshKey] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    // connect-info is the source of truth for URLs/endpoints; the local
    // fallback below keeps the tab usable if it fails. Collections are
    // best-effort — snippets fall back to a placeholder table.
    const [infoRes, keysRes, tablesRes] = await Promise.allSettled([
      api.getConnectInfo(token, projectId),
      api.listAPIKeys(token, projectId),
      api.listCollections(token, projectId),
    ]);
    if (infoRes.status === "fulfilled") setInfo(infoRes.value);
    if (keysRes.status === "fulfilled") {
      setKeys(keysRes.value);
    } else {
      setKeys([]);
      setError(keysRes.reason instanceof Error ? keysRes.reason.message : "Failed to load API keys");
    }
    if (tablesRes.status === "fulfilled") {
      const names = (tablesRes.value ?? []).map((c) => c.name).filter(Boolean);
      setTables(names);
      setTable((current) => current || names[0] || "");
    }
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function createKey() {
    setCreating(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.createAPIKey(token, projectId, "connect-quickstart");
      setFreshKey(res.plaintext);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create API key");
    } finally {
      setCreating(false);
    }
  }

  const apiBaseUrl = useMemo(
    () =>
      info?.api_base_url ??
      resolveApiBaseUrl({
        envUrl: process.env.NEXT_PUBLIC_OPENBASE_API_URL,
        origin: typeof window === "undefined" ? "" : window.location.origin,
      }),
    [info?.api_base_url]
  );
  const wsUrl = info?.realtime_url ?? realtimeUrl(apiBaseUrl);

  const apiKey = freshKey ?? API_KEY_PLACEHOLDER;
  const params = { apiBaseUrl, apiKey, table: table || TABLE_PLACEHOLDER, wsUrl };
  const activeKeys = info?.active_api_keys ?? (keys ?? []).filter((k) => !k.revoked_at).length;
  // Prefer server truth; fall back to the project context while it loads.
  const connected = info?.has_connection ?? hasConnection;
  const engineLabel = info?.engine ?? engine;
  const realtimeAvailable = info?.supports_realtime
    ? info.supports_realtime === "native"
    : supportsRealtime;
  const directDb = directDbGuidance(info?.mode);

  if (keys === null) {
    return (
      <div className="space-y-4">
        <FormSkeleton label="Loading connect details" />
        <ListSkeleton rows={3} label="Loading connect details" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && <ErrorBanner message={error} />}

      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle>Connection parameters</CardTitle>
            {engineLabel && <Badge variant="secondary">{engineLabel}</Badge>}
            <Badge variant={connected ? "success" : "muted"}>
              {connected ? "connected" : "no database"}
            </Badge>
          </div>
          <CardDescription>
            Point your app at these values. Every request is scoped to this project by its
            API key.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <CopyField label="API URL" value={apiBaseUrl} />
          <CopyField
            label="Realtime URL"
            value={wsUrl}
            hint={
              realtimeAvailable
                ? undefined
                : "This engine has no native change stream — realtime is unavailable."
            }
          />
          <CopyField label="Project ID" value={projectId} hint="For your own bookkeeping; requests are scoped by key." />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>API key</CardTitle>
          <CardDescription>
            Keys are stored hashed, so an existing key can never be shown again. Create a new
            one to get a token you can paste into the snippets below.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {freshKey ? (
            <CopyField
              label="New key (shown once)"
              value={freshKey}
              hint="Store it in your app's environment now — it will not be shown again."
            />
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <Button onClick={createKey} loading={creating}>
                Create API key
              </Button>
              <span className="text-label text-muted-foreground">
                {activeKeys === 0
                  ? "No active keys yet."
                  : `${activeKeys} active key${activeKeys === 1 ? "" : "s"} — snippets use a placeholder.`}
              </span>
            </div>
          )}
          <p className="text-label text-muted-foreground">
            Manage and revoke keys on the{" "}
            <Link href={`/orgs/${orgId}/projects/${projectId}/api`} className="text-foreground underline decoration-border decoration-1 underline-offset-2 transition-colors hover:decoration-foreground">
              API Keys
            </Link>{" "}
            tab.
          </p>
        </CardContent>
      </Card>

      {!connected && (
        <EmptyState
          title="No database attached yet"
          hint="These endpoints answer once the project has a database."
          action={
            <Button asChild variant="outline">
              <Link href={`/orgs/${orgId}/projects/${projectId}/db-source`}>Go to DB Source</Link>
            </Button>
          }
        />
      )}

      <section>
        <div className="mb-2 flex flex-wrap items-end justify-between gap-3">
          <h2 className="text-caption font-w510 text-foreground-strong">Client libraries</h2>
          {tables.length > 0 && (
            <div className="flex items-center gap-2">
              <Label htmlFor="connect-table" className="mb-0 text-label text-muted-foreground">
                Example table
              </Label>
              <select
                id="connect-table"
                value={table}
                onChange={(e) => setTable(e.target.value)}
                className="h-8 rounded-md border border-input bg-foreground/[0.02] px-2 text-label text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0"
              >
                {tables.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
        <Tabs defaultValue={CLIENT_SNIPPETS[0].id}>
          <TabsList className="flex-wrap">
            {CLIENT_SNIPPETS.map((s) => (
              <TabsTrigger key={s.id} value={s.id}>
                {s.label}
              </TabsTrigger>
            ))}
          </TabsList>
          {CLIENT_SNIPPETS.map((s) => (
            <TabsContent key={s.id} value={s.id}>
              <CodeBlock language={s.language} label={`${s.label} example`} code={s.build(params)} />
            </TabsContent>
          ))}
        </Tabs>
        {tables.length === 0 && (
          <p className="mt-2 text-label text-muted-foreground">
            Replace <code className="rounded-sm bg-foreground/[0.06] px-1 font-mono text-foreground">{TABLE_PLACEHOLDER}</code> with one of your tables.
          </p>
        )}
      </section>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Realtime</h2>
        {realtimeAvailable ? (
          <CodeBlock language="javascript" label="Realtime example" code={realtimeSnippet(params)} />
        ) : (
          <EmptyState
            title="Realtime unavailable on this engine"
            hint="Only engines advertising a native change stream can push live updates. REST polling still works."
          />
        )}
      </section>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Environment</h2>
        <CodeBlock language="dotenv" label="env example" code={envSnippet({ apiBaseUrl, apiKey })} />
        <p className="mt-2 text-label text-muted-foreground">
          Keep the key server-side. It carries full read/write access to this project&apos;s data,
          so never ship it in a browser bundle (no <code className="rounded-sm bg-foreground/[0.06] px-1 font-mono text-foreground">NEXT_PUBLIC_</code> prefix).
        </p>
      </section>

      <section>
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Direct database access (ORMs)</h2>
        <div className="max-w-xl rounded-lg border border-border bg-card p-4">
          <p className="text-caption font-w510 text-foreground-strong">{directDb.title}</p>
          <p className="mt-1.5 text-label text-muted-foreground">{directDb.body}</p>
        </div>
      </section>

      {info && (
        <section>
          <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Endpoint reference</h2>
          <div className="max-w-xl space-y-1.5 rounded-lg border border-border bg-card p-4 text-label">
            {[
              ["List tables", info.endpoints.list_tables],
              ["Read rows", info.endpoints.query_rows],
              ["Table schema", info.endpoints.table_schema],
              ["Insert row", info.endpoints.insert_row],
              ["Update row", info.endpoints.update_row],
              ["Delete row", info.endpoints.delete_row],
              ["Realtime", info.endpoints.realtime],
            ].map(([title, path]) => (
              <div key={title} className="flex flex-wrap items-baseline gap-2">
                <span className="w-24 shrink-0 text-muted-foreground">{title}</span>
                <code className="min-w-0 break-all font-mono text-foreground">{path}</code>
              </div>
            ))}
            <p className="pt-2 text-muted-foreground">
              Auth: <code className="rounded-sm bg-foreground/[0.06] px-1 font-mono text-foreground">{info.auth_header}</code>
            </p>
          </div>
        </section>
      )}
    </div>
  );
}
