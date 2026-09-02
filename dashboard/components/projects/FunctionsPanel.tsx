"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import {
  Badge,
  Button,
  EmptyState,
  ErrorBanner,
  Input,
  Label,
  Spinner,
} from "@/components/ui";
import type { Function } from "@/lib/types";

const TEMPLATES: Record<string, string> = {
  node: `exports.handler = async (event) => {
  // event = { trigger_id, project_id, collection, event, data }
  return { ok: true, data: event.data };
};`,
  python: `def handler(event):
    # event = { trigger_id, project_id, collection, event, data }
    return {"ok": True, "data": event["data"]}
`,
};

export function FunctionsPanel({ projectId }: { projectId: string }) {
  const [functions, setFunctions] = useState<Function[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [runtime, setRuntime] = useState<Function["runtime"]>("node");
  const [source, setSource] = useState(TEMPLATES.node);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      setFunctions(await api.listFunctions(token, projectId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load functions");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  function pickRuntime(rt: Function["runtime"]) {
    setRuntime(rt);
    if (!source || source === TEMPLATES.node || source === TEMPLATES.python) {
      setSource(TEMPLATES[rt]);
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.createFunction(token, projectId, {
        name: name.trim(),
        runtime,
        source,
      });
      setName("");
      setSource(TEMPLATES.node);
      setRuntime("node");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create function");
    } finally {
      setCreating(false);
    }
  }

  async function remove(id: string) {
    if (!window.confirm("Delete this function? Triggers referencing it will fail.")) return;
    setDeleting(id);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteFunction(token, projectId, id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete function");
    } finally {
      setDeleting(null);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading functions…
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">New Function</h2>
        <form onSubmit={create} className="max-w-3xl space-y-3">
          <div className="flex items-end gap-3">
            <div className="flex-1">
              <Label htmlFor="fn-name">Name</Label>
              <Input
                id="fn-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. send-welcome-email"
              />
            </div>
            <div>
              <Label>Runtime</Label>
              <div className="flex gap-1">
                <button
                  type="button"
                  onClick={() => pickRuntime("node")}
                  className={`rounded-md px-3 py-2 text-sm font-medium ${
                    runtime === "node"
                      ? "bg-brand-600 text-white"
                      : "bg-slate-100 text-slate-600 hover:bg-slate-200"
                  }`}
                >
                  Node
                </button>
                <button
                  type="button"
                  onClick={() => pickRuntime("python")}
                  className={`rounded-md px-3 py-2 text-sm font-medium ${
                    runtime === "python"
                      ? "bg-brand-600 text-white"
                      : "bg-slate-100 text-slate-600 hover:bg-slate-200"
                  }`}
                >
                  Python
                </button>
              </div>
            </div>
          </div>
          <div>
            <Label htmlFor="fn-source">Source</Label>
            <textarea
              id="fn-source"
              value={source}
              onChange={(e) => setSource(e.target.value)}
              rows={10}
              spellCheck={false}
              className="w-full rounded-lg border border-slate-300 bg-slate-50 p-3 font-mono text-xs text-slate-800 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
          </div>
          <Button type="submit" loading={creating}>
            Create Function
          </Button>
        </form>
      </section>

      {error && <ErrorBanner message={error} />}

      <section>
        <h2 className="mb-2 text-sm font-semibold text-slate-800">Functions</h2>
        {functions.length === 0 ? (
          <EmptyState
            title="No functions yet"
            hint="Write a function above, then attach it to a trigger on the Triggers tab."
          />
        ) : (
          <div className="max-w-3xl space-y-2">
            {functions.map((f) => (
              <div
                key={f.id}
                className="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-4 py-3"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-slate-800">{f.name}</span>
                    <Badge tone={f.runtime === "node" ? "green" : "blue"}>{f.runtime}</Badge>
                  </div>
                  <div className="mt-0.5 text-xs text-slate-500">
                    created {new Date(f.created_at).toLocaleDateString()}
                  </div>
                </div>
                <Button variant="danger" onClick={() => remove(f.id)} loading={deleting === f.id}>
                  Delete
                </Button>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
