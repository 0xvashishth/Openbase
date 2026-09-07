"use client";

import { useCallback, useEffect, useState } from "react";
import { authToken } from "@/components/AuthProvider";
import { api } from "@/lib/api";
import type { Invite, OrgRole } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { ToastProvider, useToast } from "@/components/ui/toast";

function InvitesPanelInner({ orgId }: { orgId: string }) {
  const toast = useToast();
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    setLoading(true);
    try {
      const list = await api.listInvites(token, orgId);
      setInvites(list);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to load invites");
    } finally {
      setLoading(false);
    }
  }, [orgId, toast]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function revoke(id: string) {
    const token = authToken();
    if (!token) return;
    try {
      await api.revokeInvite(token, orgId, id);
      toast.success("Invite revoked");
      void refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Revoke failed");
    }
  }

  const pending = invites.filter((i) => !i.accepted_at);

  return (
    <div className="space-y-3">
      <h3 className="text-heading-sm font-w510 text-foreground-strong">Pending invites</h3>
      {loading && <p className="text-body-sm text-muted-foreground">Loading…</p>}
      {!loading && pending.length === 0 && (
        <p className="text-body-sm text-muted-foreground">No pending invites.</p>
      )}
      <div className="space-y-2">
        {pending.map((inv) => (
          <div
            key={inv.id}
            className="flex items-center justify-between rounded-md border border-border bg-background px-3 py-2"
          >
            <div>
              <p className="text-body-sm text-foreground-strong">{inv.email}</p>
              <p className="text-caption text-muted-foreground">
                {inv.role} · expires {new Date(inv.expires_at).toLocaleDateString()}
              </p>
            </div>
            <Button variant="ghost" onClick={() => revoke(inv.id)}>
              Revoke
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}

export function InvitesPanel({ orgId }: { orgId: string }) {
  return (
    <ToastProvider>
      <InvitesPanelInner orgId={orgId} />
    </ToastProvider>
  );
}

// Helper to send an invite (used by MembersPanel).
export async function sendOrgInvite(
  token: string,
  orgId: string,
  email: string,
  role: OrgRole
): Promise<Invite> {
  return api.createInvite(token, orgId, email, role);
}
