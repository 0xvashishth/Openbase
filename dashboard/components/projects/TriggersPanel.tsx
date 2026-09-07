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
import { ConfirmDialog } from "@/components/ui/dialog";
import { ToolPageSkeleton } from "@/components/ui/skeletons";
import { SegmentedOption } from "@/components/ui/segmented";
import { useToast } from "@/components/ui/toast";
import type { Function, Trigger, TriggerActionType, TriggerEvent, WebhookDelivery } from "@/lib/types";

const EVENTS: { value: TriggerEvent; label: string }[] = [
  { value: "insert", label: "Insert" },
  { value: "update", label: "Update" },
  { value: "delete", label: "Delete" },
];

export function TriggersPanel({ projectId }: { projectId: string }) {
  const toast = useToast();
  const [triggers, setTriggers] = useState<Trigger[]>([]);
  const [functions, setFunctions] = useState<Function[]>([]);
  const [collections, setCollections] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [deliveryFilter, setDeliveryFilter] = useState("");

  const [name, setName] = useState("");
  const [collection, setCollection] = useState("");
  const [customCollection, setCustomCollection] = useState("");
  const [event, setEvent] = useState<TriggerEvent>("insert");
  const [actionType, setActionType] = useState<TriggerActionType>("webhook");
  const [target, setTarget] = useState("");
  const [creating, setCreating] = useState(false);
  const [toggling, setToggling] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Trigger | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    try {
      const [trigs, fns, dlv] = await Promise.all([
        api.listTriggers(token, projectId),
        api.listFunctions(token, projectId),
        api.listDeliveries(token, projectId, undefined, 50).catch(() => [] as WebhookDelivery[]),
      ]);
      setTriggers(trigs);
      setFunctions(fns);
      setDeliveries(dlv);
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
      toast.success("Trigger created", `${name.trim()} will fire on ${event} on ${col}.`);
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
      toast.success(t.enabled ? "Trigger disabled" : "Trigger enabled", t.name);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update trigger");
    } finally {
      setToggling(null);
    }
  }

  async function remove() {
    const t = pendingDelete;
    if (!t) return;
    setDeleting(t.id);
    setError(null);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteTrigger(token, projectId, t.id);
      setPendingDelete(null);
      toast.success("Trigger deleted", t.name);
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
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">New Trigger</h2>
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
                  className="h-8 w-full rounded-md border border-input bg-foreground/[0.02] px-2.5 text-caption text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0"
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
                <SegmentedOption key={ev.value} selected={event === ev.value} onClick={() => setEvent(ev.value)} className="h-8">
                  {ev.label}
                </SegmentedOption>
              ))}
            </div>
          </div>

          <div>
            <Label>Action</Label>
            <div className="grid gap-3 sm:grid-cols-[200px_1fr]">
              <div className="flex gap-1">
                <SegmentedOption selected={actionType === "function"} onClick={() => setActionType("function")} className="h-8 flex-1">
                  Function
                </SegmentedOption>
                <SegmentedOption selected={actionType === "webhook"} onClick={() => setActionType("webhook")} className="h-8 flex-1">
                  Webhook
                </SegmentedOption>
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
                  className="h-8 w-full rounded-md border border-input bg-foreground/[0.02] px-2.5 text-caption text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0"
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
        <h2 className="mb-2 text-caption font-w510 text-foreground-strong">Triggers</h2>
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
                    <span className="truncate text-caption font-w510 text-foreground-strong">{t.name}</span>
                    {t.enabled ? (
                      <StatusBadge tone="success">enabled</StatusBadge>
                    ) : (
                      <StatusBadge tone="muted">disabled</StatusBadge>
                    )}
                  </div>
                  <div className="mt-1 text-label text-muted-foreground">
                    when{" "}
                    <span className="font-mono text-foreground">{t.event}</span> on{" "}
                    <span className="font-mono text-foreground">{t.collection}</span>{" "}
                    {t.action_type === "function" ? "run" : "call"}{" "}
                    <span className="font-mono text-foreground">{targetLabel(t)}</span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button variant="ghost" onClick={() => toggle(t)} loading={toggling === t.id}>
                    {t.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="danger" onClick={() => setPendingDelete(t)} loading={deleting === t.id}>
                    Delete
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={`Delete trigger "${pendingDelete?.name ?? ""}"?`}
        description="The trigger stops firing immediately. Its target function or webhook is left in place."
        confirmLabel="Delete trigger"
        loading={deleting !== null}
        onConfirm={remove}
      />

      <section>
        <div className="mb-2 flex flex-wrap items-center gap-3">
          <h2 className="text-caption font-w510 text-foreground-strong">Delivery log</h2>
          {triggers.length > 0 && (
            <select
              aria-label="Filter deliveries by trigger"
              value={deliveryFilter}
              onChange={(e) => setDeliveryFilter(e.target.value)}
              className="h-8 rounded-md border border-input bg-foreground/[0.02] px-2.5 text-caption text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-0"
            >
              <option value="">All triggers</option>
              {triggers
                .filter((t) => t.action_type === "webhook")
                .map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                  </option>
                ))}
            </select>
          )}
          <Button variant="ghost" onClick={() => void load()}>
            Refresh
          </Button>
        </div>
        <p className="mb-3 max-w-3xl text-label text-muted-foreground">
          Every webhook delivery is signed with your project&apos;s secret — verify the{" "}
          <span className="font-mono">X-Openbase-Signature: sha256=…</span> header
          (HMAC-SHA256 of the raw body) on receipt.
        </p>
        {(() => {
          const shown = deliveryFilter
            ? deliveries.filter((d) => d.trigger_id === deliveryFilter)
            : deliveries;
          if (shown.length === 0) {
            return (
              <EmptyState
                title="No deliveries yet"
                hint="Fire a trigger (insert a row on a watched table) and its webhook attempts will appear here with status, latency and errors."
              />
            );
          }
          const triggerName = (id?: string) =>
            triggers.find((t) => t.id === id)?.name ?? id?.slice(0, 8) ?? "—";
          return (
            <div className="max-w-3xl space-y-2">
              {shown.map((d) => (
                <div
                  key={d.id}
                  className="rounded-lg border border-border bg-card px-4 py-3"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    {d.ok ? (
                      <StatusBadge tone="success">{d.status_code ?? "delivered"}</StatusBadge>
                    ) : (
                      <StatusBadge tone="destructive">
                        {d.status_code ?? "failed"}
                      </StatusBadge>
                    )}
                    <span className="truncate text-caption font-w510 text-foreground-strong">
                      {triggerName(d.trigger_id)}
                    </span>
                    <span className="text-label text-muted-foreground">
                      {d.event} on <span className="font-mono">{d.collection}</span>
                    </span>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-label text-muted-foreground">
                    <span className="font-mono">{d.target_url}</span>
                    <span>
                      {d.attempts} {d.attempts === 1 ? "attempt" : "attempts"} ·{" "}
                      {d.duration_ms}ms · {new Date(d.created_at).toLocaleString()}
                    </span>
                  </div>
                  {!d.ok && d.error && (
                    <div className="mt-1 font-mono text-label text-destructive">{d.error}</div>
                  )}
                </div>
              ))}
            </div>
          );
        })()}
      </section>
    </div>
  );
}
