"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, EmptyState, ErrorBanner, Label } from "@/components/ui";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ListSkeleton } from "@/components/ui/skeletons";
import { useToast } from "@/components/ui/toast";
import type { AuthHookView, Function } from "@/lib/types";

const EVENTS: Array<{ id: AuthHookView["event"]; hint: string }> = [
  { id: "before-user-created", hint: "May reject signup ({error}) or merge user_metadata." },
  { id: "after-user-created", hint: "Notification-style; failures never block signup." },
  { id: "before-token-issued", hint: "May merge custom_claims into the access token." },
];

export function AuthHooksPanel({ projectId }: { projectId: string }) {
  const toast = useToast();
  const [hooks, setHooks] = useState<AuthHookView[]>([]);
  const [functions, setFunctions] = useState<Function[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [event, setEvent] = useState<AuthHookView["event"]>("before-user-created");
  const [functionId, setFunctionId] = useState("");
  const [failOpen, setFailOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  async function load() {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    setError(null);
    try {
      const [h, fns] = await Promise.all([
        api.listAuthHooks(token, projectId),
        api.listFunctions(token, projectId),
      ]);
      setHooks(h);
      setFunctions(fns);
      if (!functionId && fns.length > 0) setFunctionId(fns[0].id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load hooks");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const fnName = (id: string) => functions.find((f) => f.id === id)?.name ?? id.slice(0, 8);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!functionId) {
      setError("Pick a function to run (create one under Functions first).");
      return;
    }
    const token = authToken();
    if (!token) return;
    setSaving(true);
    setError(null);
    try {
      await api.upsertAuthHook(token, projectId, { event, function_id: functionId, fail_open: failOpen });
      toast.success("Hook saved");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save hook");
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!pendingDelete) return;
    const token = authToken();
    if (!token) return;
    try {
      await api.deleteAuthHook(token, projectId, pendingDelete);
      setPendingDelete(null);
      toast.success("Hook removed");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to remove hook");
    }
  }

  if (loading) return <ListSkeleton />;

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} />}
      {hooks.length === 0 ? (
        <EmptyState
          title="No auth hooks"
          hint="Run a project function when users sign up or tokens are issued — e.g. blocking disposable emails or stamping tenants into tokens."
        />
      ) : (
        <ul className="divide-y divide-border rounded-md border">
          {hooks.map((h) => (
            <li key={h.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-body-sm font-medium">{h.event}</p>
                <p className="truncate text-caption text-muted-foreground">→ {fnName(h.function_id)}</p>
              </div>
              <Badge>{h.fail_open ? "fail-open" : "fail-closed"}</Badge>
              <Button size="sm" variant="destructive" onClick={() => setPendingDelete(h.event)}>
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form className="grid gap-3 rounded-md border p-3" onSubmit={save}>
        <div className="grid gap-1">
          <Label htmlFor="hook-event">Event</Label>
          <select
            id="hook-event"
            className="rounded-md border bg-transparent px-2 py-1.5 text-body-sm"
            value={event}
            onChange={(e) => setEvent(e.target.value as AuthHookView["event"])}
          >
            {EVENTS.map(({ id, hint }) => (
              <option key={id} value={id} title={hint}>
                {id}
              </option>
            ))}
          </select>
          <p className="text-caption text-muted-foreground">
            {EVENTS.find((x) => x.id === event)?.hint}
          </p>
        </div>
        <div className="grid gap-1">
          <Label htmlFor="hook-function">Function</Label>
          <select
            id="hook-function"
            className="rounded-md border bg-transparent px-2 py-1.5 text-body-sm"
            value={functionId}
            onChange={(e) => setFunctionId(e.target.value)}
          >
            {functions.length === 0 && <option value="">No functions yet — create one first</option>}
            {functions.map((f) => (
              <option key={f.id} value={f.id}>
                {f.name} ({f.runtime})
              </option>
            ))}
          </select>
        </div>
        <label className="flex items-center gap-2 text-body-sm">
          <input type="checkbox" checked={failOpen} onChange={(e) => setFailOpen(e.target.checked)} />
          Fail open (auth continues if the function errors)
        </label>
        <div>
          <Button type="submit" disabled={saving}>
            {saving ? "Saving…" : "Save hook"}
          </Button>
        </div>
      </form>
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title="Remove this hook?"
        description="The event stops invoking the function immediately."
        confirmLabel="Remove"
        onConfirm={remove}
      />
    </div>
  );
}
