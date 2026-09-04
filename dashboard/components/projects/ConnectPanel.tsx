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
import type { APIKeyView } from "@/lib/types";

/**
 * Connect tab: how an external app talks TO Openbase (outbound). The DB Source
 * tab is the opposite direction — attaching the project's backing database.
 *
 * Keys are hash-only at rest, so a freshly created key is the only time we can
 * show a real token. Existing keys fall back to a placeholder in snippets.
 */
export function ConnectPanel({ projectId }: { projectId: string }) {
  const { orgId, engine, hasConnection, supportsRealtime } = useProject();
  const [keys, setKeys] = useState<APIKeyView[] | null>(null);
  const [freshKey, setFreshKey] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    try {
      setKeys(await api.listAPIKeys(token, projectId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load API keys");
      setKeys([]);
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
      resolveApiBaseUrl({
        envUrl: process.env.NEXT_PUBLIC_OPENBASE_API_URL,
        origin: typeof window === "undefined" ? "" : window.location.origin,
      }),
    []
  );

  const apiKey = freshKey ?? API_KEY_PLACEHOLDER;
  const params = { apiBaseUrl, apiKey, table: TABLE_PLACEHOLDER };
  const activeKeys = (keys ?? []).filter((k) => !k.revoked_at);

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
            {engine && <Badge variant="secondary">{engine}</Badge>}
            <Badge variant={hasConnection ? "success" : "warning"}>
              {hasConnection ? "connected" : "no database"}
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
            value={realtimeUrl(apiBaseUrl)}
            hint={
              supportsRealtime
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
                {activeKeys.length === 0
                  ? "No active keys yet."
                  : `${activeKeys.length} active key${activeKeys.length === 1 ? "" : "s"} — snippets use a placeholder.`}
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

      {!hasConnection && (
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
    </div>
  );
}
