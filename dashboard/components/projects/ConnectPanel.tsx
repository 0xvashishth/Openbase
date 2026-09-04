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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FormSkeleton, ListSkeleton } from "@/components/ui/skeletons";
import { useProject } from "@/lib/project-context";
import {
  API_KEY_PLACEHOLDER,
  TABLE_PLACEHOLDER,
  curlSnippet,
  envSnippet,
  jsSnippet,
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
  const [freshKey, setFreshKey] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    // connect-info is the source of truth for URLs/endpoints; the local
    // fallback below keeps the tab usable if it fails.
    const [infoRes, keysRes] = await Promise.allSettled([
      api.getConnectInfo(token, projectId),
      api.listAPIKeys(token, projectId),
    ]);
    if (infoRes.status === "fulfilled") setInfo(infoRes.value);
    if (keysRes.status === "fulfilled") {
      setKeys(keysRes.value);
    } else {
      setKeys([]);
      setError(keysRes.reason instanceof Error ? keysRes.reason.message : "Failed to load API keys");
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
  const params = { apiBaseUrl, apiKey, table: TABLE_PLACEHOLDER, wsUrl };
  const activeKeys = info?.active_api_keys ?? (keys ?? []).filter((k) => !k.revoked_at).length;
  // Prefer server truth; fall back to the project context while it loads.
  const connected = info?.has_connection ?? hasConnection;
  const engineLabel = info?.engine ?? engine;
  const realtimeAvailable = info?.supports_realtime
    ? info.supports_realtime === "native"
    : supportsRealtime;

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
            <Badge variant={connected ? "success" : "warning"}>
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
              <span className="text-xs text-muted-foreground">
                {activeKeys === 0
                  ? "No active keys yet."
                  : `${activeKeys} active key${activeKeys === 1 ? "" : "s"} — snippets use a placeholder.`}
              </span>
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            Manage and revoke keys on the{" "}
            <Link href={`/orgs/${orgId}/projects/${projectId}/api`} className="underline hover:text-foreground">
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
        <h2 className="mb-2 text-sm font-semibold text-foreground">Quickstart</h2>
        <Tabs defaultValue="curl">
          <TabsList>
            <TabsTrigger value="curl">cURL</TabsTrigger>
            <TabsTrigger value="js">JavaScript</TabsTrigger>
            <TabsTrigger value="realtime">Realtime</TabsTrigger>
            <TabsTrigger value="env">.env</TabsTrigger>
          </TabsList>
          <TabsContent value="curl">
            <CodeBlock language="bash" label="cURL example" code={curlSnippet(params)} />
          </TabsContent>
          <TabsContent value="js">
            <CodeBlock language="javascript" label="JavaScript example" code={jsSnippet(params)} />
          </TabsContent>
          <TabsContent value="realtime">
            <CodeBlock language="javascript" label="Realtime example" code={realtimeSnippet(params)} />
          </TabsContent>
          <TabsContent value="env">
            <CodeBlock language="dotenv" label="env example" code={envSnippet({ apiBaseUrl, apiKey })} />
          </TabsContent>
        </Tabs>
        <p className="mt-2 text-xs text-muted-foreground">
          Replace <code className="font-mono">{TABLE_PLACEHOLDER}</code> with one of your tables.
        </p>
      </section>

      {info && (
        <section>
          <h2 className="mb-2 text-sm font-semibold text-foreground">Endpoint reference</h2>
          <div className="max-w-xl space-y-1.5 rounded-xl border border-border bg-card p-4 text-xs">
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
              Auth: <code className="font-mono">{info.auth_header}</code>
            </p>
          </div>
        </section>
      )}
    </div>
  );
}
