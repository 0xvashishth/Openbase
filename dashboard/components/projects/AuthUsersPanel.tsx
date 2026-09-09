"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Button, EmptyState, ErrorBanner, Input, Label } from "@/components/ui";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ListSkeleton } from "@/components/ui/skeletons";
import { useToast } from "@/components/ui/toast";
import type { ProjectUserView } from "@/lib/types";

function Badges({ user }: { user: ProjectUserView }) {
  return (
    <span className="flex flex-wrap gap-1">
      {user.is_anonymous ? (
        <Badge>anonymous</Badge>
      ) : (
        <Badge>{user.email_confirmed_at ? "confirmed" : "unconfirmed"}</Badge>
      )}
      {user.banned_until && <Badge variant="destructive">banned</Badge>}
    </span>
  );
}

export function AuthUsersPanel({ projectId }: { projectId: string }) {
  const toast = useToast();
  const [users, setUsers] = useState<ProjectUserView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  async function load(q?: string) {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    setError(null);
    try {
      const res = await api.listProjectUsers(token, projectId, q || undefined);
      setUsers(res.users);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  async function toggleBan(user: ProjectUserView) {
    const token = authToken();
    if (!token) return;
    setBusy(user.id);
    setError(null);
    try {
      await api.updateProjectUser(token, projectId, user.id, {
        banned: !user.banned_until,
        ban_duration: "720h",
      });
      toast.success(user.banned_until ? "User unbanned" : "User banned for 30 days");
      await load(search);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update user");
    } finally {
      setBusy(null);
    }
  }

  async function confirmEmail(user: ProjectUserView) {
    const token = authToken();
    if (!token) return;
    setBusy(user.id);
    setError(null);
    try {
      await api.updateProjectUser(token, projectId, user.id, { confirm_email: true });
      toast.success("Email marked confirmed");
      await load(search);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to confirm email");
    } finally {
      setBusy(null);
    }
  }

  async function remove() {
    const id = pendingDelete;
    if (!id) return;
    const token = authToken();
    if (!token) return;
    setBusy(id);
    try {
      await api.deleteProjectUser(token, projectId, id);
      setPendingDelete(null);
      toast.success("User deleted");
      await load(search);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete user");
    } finally {
      setBusy(null);
    }
  }

  if (loading) return <ListSkeleton />;

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} />}
      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          load(search);
        }}
      >
        <div className="grid flex-1 gap-1">
          <Label htmlFor="auth-user-search">Search by email</Label>
          <Input
            id="auth-user-search"
            placeholder="name@example.com"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <Button type="submit">Search</Button>
      </form>
      {users.length === 0 ? (
        <EmptyState
          title="No end users yet"
          hint="Users appear here after they sign up through your app's SDK (signUp, OAuth or anonymous sign-in)."
        />
      ) : (
        <ul className="divide-y divide-border rounded-md border">
          {users.map((u) => (
            <li key={u.id} className="flex flex-wrap items-center gap-3 px-3 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-body-sm font-medium">
                  {u.email ?? u.phone ?? `anonymous · ${u.id.slice(0, 8)}`}
                </p>
                <p className="truncate text-caption text-muted-foreground">{u.id}</p>
              </div>
              <Badges user={u} />
              {!u.is_anonymous && !u.email_confirmed_at && (
                <Button size="sm" variant="outline" disabled={busy === u.id} onClick={() => confirmEmail(u)}>
                  Confirm email
                </Button>
              )}
              <Button size="sm" variant="outline" disabled={busy === u.id} onClick={() => toggleBan(u)}>
                {u.banned_until ? "Unban" : "Ban"}
              </Button>
              <Button size="sm" variant="destructive" disabled={busy === u.id} onClick={() => setPendingDelete(u.id)}>
                Delete
              </Button>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title="Delete this user?"
        description="Their sessions and identities are removed too. This cannot be undone."
        confirmLabel="Delete"
        onConfirm={remove}
      />
    </div>
  );
}
