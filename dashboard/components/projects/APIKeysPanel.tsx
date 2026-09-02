"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, EmptyState, ErrorBanner, Input, Label, Spinner, Badge } from "@/components/ui";
import type { APIKeyView } from "@/lib/types";

export function APIKeysPanel({ projectId }: { projectId: string }) {
  const [keys, setKeys] = useState<APIKeyView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const [newKey, setNewKey] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      setKeys(await api.listAPIKeys(token, projectId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load keys");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    setError(null);
    setNewKey(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      const res = await api.createAPIKey(token, projectId, name.trim());
      setNewKey(res.plaintext);
      setName("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create key");
    } finally {
      setCreating(false);
    }
  }

  async function revoke(id: string) {
    if (!window.confirm("Revoke this API key? This cannot be undone.")) return;
    setRevoking(id);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.revokeAPIKey(token, projectId, id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to revoke key");
    } finally {
      setRevoking(null);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading API keys…
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Create API Key</h2>
        <form onSubmit={create} className="flex max-w-xl items-end gap-3">
          <div className="flex-1">
            <Label htmlFor="key-name">Name</Label>
            <Input
              id="key-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. production-app"
            />
          </div>
          <Button type="submit" loading={creating}>
            Create
          </Button>
        </form>
        {newKey && (
          <div className="mt-3 max-w-xl rounded-md border border-amber-200 bg-amber-50 p-3">
            <p className="text-xs font-medium text-amber-800">
              Copy this key now — it won't be shown again:
            </p>
            <code className="mt-1 block break-all rounded bg-amber-100 p-2 text-xs text-amber-900">
              {newKey}
            </code>
          </div>
        )}
      </section>

      {error && <ErrorBanner message={error} />}

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Existing Keys</h2>
        {keys.length === 0 ? (
          <EmptyState
            title="No API keys"
            hint="Create one above to access your project's auto-generated REST API."
          />
        ) : (
          <div className="max-w-xl space-y-2">
            {keys.map((k) => (
              <div
                key={k.id}
                className="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-4 py-3"
              >
                <div>
                  <span className="text-sm font-medium text-slate-800">{k.name}</span>
                  <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-500">
                    <span>created {new Date(k.created_at).toLocaleDateString()}</span>
                    {k.revoked_at && <Badge tone="red">revoked</Badge>}
                  </div>
                </div>
                {!k.revoked_at && (
                  <Button
                    variant="danger"
                    onClick={() => revoke(k.id)}
                    loading={revoking === k.id}
                  >
                    Revoke
                  </Button>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="max-w-xl">
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Using the API</h2>
        <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 text-xs text-slate-600 space-y-2">
          <p>
            <strong>List tables:</strong>{" "}
            <code className="rounded bg-slate-100 px-1">GET /v1/api/tables</code>
          </p>
          <p>
            <strong>Query rows:</strong>{" "}
            <code className="rounded bg-slate-100 px-1">GET /v1/api/{'{table}'}?limit=10&order_by=id</code>
          </p>
          <p>
            <strong>Insert row:</strong>{" "}
            <code className="rounded bg-slate-100 px-1">POST /v1/api/{'{table}'}</code> with JSON body
          </p>
          <p>
            <strong>Update row:</strong>{" "}
            <code className="rounded bg-slate-100 px-1">PUT /v1/api/{'{table}'}/{'{id}'}</code>
          </p>
          <p>
            <strong>Delete row:</strong>{" "}
            <code className="rounded bg-slate-100 px-1">DELETE /v1/api/{'{table}'}/{'{id}'}</code>
          </p>
          <p className="mt-2 text-slate-400">
            Authentication:{" "}
            <code className="rounded bg-slate-100 px-1">Authorization: Bearer ob_...</code>
          </p>
        </div>
      </section>
    </div>
  );
}
