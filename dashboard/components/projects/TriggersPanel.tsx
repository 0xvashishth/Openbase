"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import {
  Button,
  EmptyState,
  ErrorBanner,
  Input,
  Label,
} from "@/components/ui";
import { StatusBadge } from "@/components/ui/badge";
import { ToolPageSkeleton } from "@/components/ui/skeletons";
import type { Function, Trigger, TriggerActionType, TriggerEvent } from "@/lib/types";

const EVENTS: { value: TriggerEvent; label: string }[] = [
  { value: "insert", label: "Insert" },
  { value: "update", label: "Update" },
  { value: "delete", label: "Delete" },
];

export function TriggersPanel({ projectId }: { projectId: string }) {
  const [triggers, setTriggers] = useState<Trigger[]>([]);
  const [functions, setFunctions] = useState<Function[]>([]);
  const [collections, setCollections] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [collection, setCollection] = useState("");
  const [customCollection, setCustomCollection] = useState("");
  const [event, setEvent] = useState<TriggerEvent>("insert");
  const [actionType, setActionType] = useState<TriggerActionType>("webhook");
  const [target, setTarget] = useState("");
  const [creating, setCreating] = useState(false);
  const [toggling, setToggling] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      const [trigs, fns] = await Promise.all([
        api.listTriggers(token, projectId),
        api.listFunctions(token, projectId),
      ]);
      setTriggers(trigs);
      setFunctions(fns);
      try {
        const cols = await api.listCollections(token, projectId);
        setCollections(cols.map((c) => c.name));
      } catch {
        setCollections([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load triggers");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const selectedCollection = collection === "__custom" ? customCollection : collection;

  async function create(e: React.FormEvent) {
    e.preventDefault();
    const col = selectedCollection.trim();
    if (!name.trim() || !col || !target.trim()) return;
    setCreating(true);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.createTrigger(token, projectId, {
        name: name.trim(),
        collection: col,
        event,
        action_type: actionType,
        action_target: target.trim(),
      });
      setName("");
      setCollection("");
      setCustomCollection("");
      setTarget("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create trigger");
    } finally {
      setCreating(false);
    }
  }

  async function toggle(t: Trigger) {
    setToggling(t.id);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.updateTrigger(token, projectId, t.id, { enabled: !t.enabled });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update trigger");
    } finally {
      setToggling(null);
    }
  }

  async function remove(t: Trigger) {
    if (!window.confirm(`Delete trigger "${t.name}"?`)) return;
    setDeleting(t.id);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteTrigger(token, projectId, t.id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete trigger");
    } finally {
      setDeleting(null);
    }
  }

  const targetLabel = (t: Trigger) =>
    t.action_type === "function" ? `→ ${t.action_target}` : t.action_target;

  if (loading) {
    return <ToolPageSkeleton label="Loading triggers" />;
  }

  return (
    <div className="space-y-4">
      <section>
        <h2 className="mb-2 text-sm font-semibold text-foreground">New Trigger</h2>
        <form onSubmit={create} className="max-w-3xl space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="trig-name">Name</Label>
              <Input
                id="trig-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. order-placed"
              />
            </div>
            <div>
              <Label htmlFor="trig-collection">Collection</Label>
              {collections.length > 0 ? (
                <select
                  id="trig-collection"
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
                  <option value="__custom">Custom name…</option>
                </select>
              ) : (
                <Input
                  id="trig-collection"
                  value={collection}
                  onChange={(e) => setCollection(e.target.value)}
                  placeholder="table name"
                />
              )}
              {collection === "__custom" && (
                <Input
                  className="mt-2"
                  value={customCollection}
                  onChange={(e) => setCustomCollection(e.target.value)}
                  placeholder="custom collection name"
                />
              )}
            </div>
          </div>

          <div>
            <Label>Event</Label>
            <div className="flex gap-1">
              {EVENTS.map((ev) => (
                <button
                  key={ev.value}
                  type="button"
                  onClick={() => setEvent(ev.value)}
                  aria-pressed={event === ev.value}
                  className={`rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                    event === ev.value
                      ? "border-primary bg-accent text-foreground"
                      : "border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  }`}
                >
                  {ev.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <Label>Action</Label>
            <div className="grid gap-3 sm:grid-cols-[200px_1fr]">
              <div className="flex gap-1">
                <button
                  type="button"
                  onClick={() => setActionType("function")}
                  aria-pressed={actionType === "function"}
                  className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                    actionType === "function"
                      ? "border-primary bg-accent text-foreground"
                      : "border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  }`}
                >
                  Function
                </button>
                <button
                  type="button"
                  onClick={() => setActionType("webhook")}
                  aria-pressed={actionType === "webhook"}
                  className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                    actionType === "webhook"
                      ? "border-primary bg-accent text-foreground"
                      : "border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  }`}
                >
                  Webhook
                </button>
              </div>
              {actionType === "webhook" ? (
                <Input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder="https://example.com/hook"
                />
              ) : (
                <select
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  className="w-full rounded-lg border border-input bg-card px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-ring"
                >
                  <option value="">Select a function…</option>
                  {functions.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name} ({f.runtime})
                    </option>
                  ))}
                </select>
              )}
            </div>
          </div>

          <Button type="submit" loading={creating}>
            Create Trigger
          </Button>
        </form>
      </section>

      {error && <ErrorBanner message={error} />}

      <section>
        <h2 className="mb-2 text-sm font-semibold text-foreground">Triggers</h2>
        {triggers.length === 0 ? (
          <EmptyState
            title="No triggers yet"
            hint="Build your first rule: when an event happens on a table, run a function or hit a webhook."
          />
        ) : (
          <div className="max-w-3xl space-y-2">
            {triggers.map((t) => (
              <div
                key={t.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">{t.name}</span>
                    {t.enabled ? (
                      <StatusBadge tone="success">enabled</StatusBadge>
                    ) : (
                      <StatusBadge tone="muted">disabled</StatusBadge>
                    )}
                  </div>
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    when{" "}
                    <span className="font-medium text-muted-foreground">{t.event}</span> on{" "}
                    <span className="font-medium text-muted-foreground">{t.collection}</span>{" "}
                    {t.action_type === "function" ? "run" : "call"}{" "}
                    <span className="font-mono text-muted-foreground">{targetLabel(t)}</span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button variant="ghost" onClick={() => toggle(t)} loading={toggling === t.id}>
                    {t.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="danger" onClick={() => remove(t)} loading={deleting === t.id}>
                    Delete
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
