"use client";

import { useCallback, useEffect, useState } from "react";
import { authToken } from "@/components/AuthProvider";
import { useAuth } from "@/components/AuthProvider";
import { api } from "@/lib/api";
import type { SessionView, User } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToastProvider, useToast } from "@/components/ui/toast";

function AccountPanelInner() {
  const { user, logout } = useAuth();
  const toast = useToast();
  const [fullName, setFullName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [sessions, setSessions] = useState<SessionView[]>([]);

  const refresh = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    try {
      const list = await api.listMySessions(token);
      setSessions(list);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load sessions");
    }
  }, [toast]);

  useEffect(() => {
    if (user) setFullName(user.full_name ?? "");
    void refresh();
  }, [user, refresh]);

  if (!user) {
    return <p className="text-body-sm text-muted-foreground">Not signed in.</p>;
  }

  async function saveProfile(e: React.FormEvent) {
    e.preventDefault();
    const token = authToken();
    if (!token) return;
    try {
      const updated: User = await api.updateMe(token, { full_name: fullName });
      // Refresh user data from server so the UI reflects the persisted state
      // immediately (rather than waiting for the next auth context refresh).
      const fresh = await api.me(token);
      setFullName(fresh.full_name ?? "");
      toast.success("Profile updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Update failed");
    }
  }

  async function savePassword(e: React.FormEvent) {
    e.preventDefault();
    const token = authToken();
    if (!token) return;
    if (newPassword.length < 8) {
      toast.error("New password must be at least 8 characters");
      return;
    }
    try {
      await api.changePassword(token, currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      toast.success("Password changed");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Password change failed");
    }
  }

  async function revokeSession(id: string) {
    const token = authToken();
    if (!token) return;
    try {
      await api.revokeMySession(token, id);
      toast.success("Session revoked");
      void refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Revoke failed");
    }
  }

  return (
    <div className="space-y-8">
      <section>
        <h2 className="mb-2 text-heading-sm font-w510 text-foreground-strong">Profile</h2>
        <form onSubmit={saveProfile} className="space-y-3">
          <div>
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" value={user.email} disabled />
          </div>
          <div>
            <Label htmlFor="fullName">Full name</Label>
            <Input
              id="fullName"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
            />
          </div>
          <Button type="submit" variant="primary">Save profile</Button>
        </form>
      </section>

      <section>
        <h2 className="mb-2 text-heading-sm font-w510 text-foreground-strong">Change password</h2>
        <form onSubmit={savePassword} className="space-y-3">
          <div>
            <Label htmlFor="current">Current password</Label>
            <Input
              id="current"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
            />
          </div>
          <div>
            <Label htmlFor="next">New password</Label>
            <Input
              id="next"
              type="password"
              autoComplete="new-password"
              minLength={8}
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
            />
          </div>
          <Button type="submit" variant="primary">Change password</Button>
        </form>
      </section>

      <section>
        <h2 className="mb-2 text-heading-sm font-w510 text-foreground-strong">Active sessions</h2>
        <p className="mb-3 text-body-sm text-muted-foreground">
          Devices where you are currently signed in. Revoking invalidates the
          session immediately.
        </p>
        <div className="space-y-2">
          {sessions.length === 0 && (
            <p className="text-body-sm text-muted-foreground">No active sessions.</p>
          )}
          {sessions.map((s) => (
            <div
              key={s.id}
              className="flex items-center justify-between rounded-md border border-border bg-background px-3 py-2"
            >
              <div>
                <p className="text-body-sm text-foreground-strong">
                  {s.user_agent || "Unknown device"}
                </p>
                <p className="text-caption text-muted-foreground">
                  {s.ip || "?"} · since {new Date(s.created_at).toLocaleString()}
                  {s.revoked_at ? " · revoked" : ""}
                </p>
              </div>
              {!s.revoked_at && (
                <Button
                  variant="ghost"
                  onClick={() => revokeSession(s.id)}
                >
                  Revoke
                </Button>
              )}
            </div>
          ))}
        </div>
        <Button variant="ghost" className="mt-3" onClick={logout}>
          Sign out this device
        </Button>
      </section>
    </div>
  );
}

export function AccountPanel() {
  return (
    <ToastProvider>
      <AccountPanelInner />
    </ToastProvider>
  );
}
