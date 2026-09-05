"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { ApiError, api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { useOrg } from "@/lib/org-context";
import { useToast } from "@/components/ui/toast";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ConfirmDialog,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ErrorBanner, Spinner } from "@/components/ui/feedback";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SegmentedOption } from "@/components/ui/segmented";
import { ListSkeleton } from "@/components/ui/skeletons";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { OrgMember, OrgRole, User } from "@/lib/types";

const ROLE_ORDER: Record<OrgRole, number> = { owner: 3, admin: 2, member: 1 };

const LAST_OWNER_REASON = "An organization must keep at least one owner.";

function initials(m: OrgMember): string {
  const source = m.full_name?.trim() || m.email;
  const parts = source.split(/[\s@._-]+/).filter(Boolean);
  return (parts[0]?.[0] ?? "?").concat(parts[1]?.[0] ?? "").toUpperCase();
}

export function MembersPanel() {
  const { orgId, role: viewerRole, can } = useOrg();
  const toast = useToast();

  const [members, setMembers] = useState<OrgMember[] | null>(null);
  const [me, setMe] = useState<User | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [busyUserId, setBusyUserId] = useState<string | null>(null);

  const [addOpen, setAddOpen] = useState(false);
  const [addEmail, setAddEmail] = useState("");
  const [addRole, setAddRole] = useState<OrgRole>("member");
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [removeTarget, setRemoveTarget] = useState<OrgMember | null>(null);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<string | null>(null);

  const [transferTarget, setTransferTarget] = useState<OrgMember | null>(null);
  const [demoteSelf, setDemoteSelf] = useState(true);
  const [transferring, setTransferring] = useState(false);
  const [transferError, setTransferError] = useState<string | null>(null);

  const canManage = can("add_member");
  const canTransfer = can("transfer_ownership");

  const load = useCallback(async () => {
    const token = authToken();
    if (!token) return;
    try {
      const list = await api.listMembers(token, orgId);
      setMembers(
        [...(list ?? [])].sort(
          (a, b) =>
            ROLE_ORDER[b.role] - ROLE_ORDER[a.role] ||
            new Date(a.joined_at).getTime() - new Date(b.joined_at).getTime()
        )
      );
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load members");
    }
  }, [orgId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Needed to mark "You" and to decide whether a row is a self-action (leave)
  // rather than a removal.
  useEffect(() => {
    const token = authToken();
    if (!token) return;
    let cancelled = false;
    api
      .me(token)
      .then((u) => !cancelled && setMe(u))
      .catch(() => !cancelled && setMe(null));
    return () => {
      cancelled = true;
    };
  }, []);

  const ownerCount = useMemo(
    () => (members ?? []).filter((m) => m.role === "owner").length,
    [members]
  );

  async function addMember(e: React.FormEvent) {
    e.preventDefault();
    setAddError(null);
    const email = addEmail.trim();
    if (!email) {
      setAddError("Enter the email address of an existing Openbase account.");
      return;
    }
    setAdding(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.addMember(token, orgId, email, addRole);
      await load();
      setAddOpen(false);
      setAddEmail("");
      setAddRole("member");
      toast.success("Member added", email);
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        // There is no mailer yet (PHASES 9.2), so an invite cannot be sent.
        // Say so plainly instead of implying one went out.
        setAddError("No Openbase account uses that email — they need to sign up first.");
      } else if (err instanceof ApiError && err.status === 409) {
        setAddError("That person is already a member of this organization.");
      } else {
        setAddError(err instanceof Error ? err.message : "Failed to add member");
      }
    } finally {
      setAdding(false);
    }
  }

  /**
   * Optimistic role change with rollback. The client-side guards below are UX;
   * the server's last-owner and owner-grant rules remain the truth, so a 409 or
   * 403 still surfaces as a toast.
   */
  async function changeRole(target: OrgMember, nextRole: OrgRole) {
    if (nextRole === target.role) return;
    const previous = members;
    setMembers((prev) =>
      (prev ?? []).map((m) => (m.user_id === target.user_id ? { ...m, role: nextRole } : m))
    );
    setBusyUserId(target.user_id);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.updateMemberRole(token, orgId, target.user_id, nextRole);
      await load();
      toast.success("Role updated", `${target.email} is now ${nextRole}`);
    } catch (err) {
      setMembers(previous);
      toast.error(
        "Could not change role",
        err instanceof Error ? err.message : "The server rejected the change"
      );
    } finally {
      setBusyUserId(null);
    }
  }

  async function removeMember() {
    if (!removeTarget) return;
    const isSelf = me?.id === removeTarget.user_id;
    setRemoveError(null);
    setRemoving(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.removeMember(token, orgId, removeTarget.user_id);
      toast.success(isSelf ? "You left the organization" : "Member removed", removeTarget.email);
      setRemoveTarget(null);
      if (isSelf) {
        // Membership is gone, so every org-scoped read now 403s.
        window.location.assign("/orgs");
        return;
      }
      await load();
    } catch (err) {
      setRemoveError(
        err instanceof ApiError && err.status === 409
          ? LAST_OWNER_REASON
          : err instanceof Error
            ? err.message
            : "Failed to remove member"
      );
    } finally {
      setRemoving(false);
    }
  }

  async function transferOwnership() {
    if (!transferTarget) return;
    setTransferError(null);
    setTransferring(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.transferOwnership(token, orgId, transferTarget.user_id, demoteSelf);
      await load();
      toast.success("Ownership transferred", transferTarget.email);
      setTransferTarget(null);
    } catch (err) {
      setTransferError(err instanceof Error ? err.message : "Failed to transfer ownership");
    } finally {
      setTransferring(false);
    }
  }

  if (members === null && !loadError) return <ListSkeleton rows={4} label="Loading members" />;
  if (loadError) return <ErrorBanner message={loadError} />;

  const rows = members ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-caption text-muted-foreground">
          {rows.length} member{rows.length === 1 ? "" : "s"}
        </p>
        {canManage && (
          <Button variant="primary" onClick={() => setAddOpen(true)}>
            Add member
          </Button>
        )}
      </div>

      <div className="rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Member</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Joined</TableHead>
              {canManage && (
                <TableHead className="w-10">
                  <span className="sr-only">Actions</span>
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((m) => {
              const isSelf = me?.id === m.user_id;
              const isLastOwner = m.role === "owner" && ownerCount <= 1;
              // An admin cannot touch an owner row at all — the server 403s, so
              // offering the control would be a lie.
              const targetsOwnerAsAdmin = m.role === "owner" && viewerRole !== "owner";
              const busy = busyUserId === m.user_id;

              return (
                <TableRow key={m.user_id}>
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <Avatar>
                        <AvatarFallback>{initials(m)}</AvatarFallback>
                      </Avatar>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className="truncate text-caption font-w510 text-foreground-strong">
                            {m.full_name || m.email}
                          </span>
                          {isSelf && <Badge variant="secondary">You</Badge>}
                        </div>
                        <span className="block truncate text-label text-muted-foreground">{m.email}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    {canManage && !targetsOwnerAsAdmin ? (
                      <RoleSelect
                        value={m.role}
                        disabled={busy || isLastOwner}
                        disabledReason={isLastOwner ? LAST_OWNER_REASON : undefined}
                        allowOwner={viewerRole === "owner"}
                        onChange={(next) => void changeRole(m, next)}
                      />
                    ) : (
                      <Badge variant="secondary">{m.role}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Date(m.joined_at).toLocaleDateString()}
                  </TableCell>
                  {canManage && (
                    <TableCell>
                      {busy ? (
                        <Spinner className="h-3.5 w-3.5 text-muted-foreground" />
                      ) : (
                        <RowMenu
                          member={m}
                          isSelf={isSelf}
                          isLastOwner={isLastOwner}
                          disabled={targetsOwnerAsAdmin}
                          canTransfer={canTransfer && !isSelf}
                          onRemove={() => {
                            setRemoveError(null);
                            setRemoveTarget(m);
                          }}
                          onTransfer={() => {
                            setTransferError(null);
                            setDemoteSelf(true);
                            setTransferTarget(m);
                          }}
                        />
                      )}
                    </TableCell>
                  )}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>

      {!canManage && (
        <p className="text-label text-muted-foreground">
          Only owners and admins can manage members.
        </p>
      )}

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <form onSubmit={addMember} className="space-y-4">
            <DialogHeader>
              <DialogTitle>Add a member</DialogTitle>
              <DialogDescription>
                The person must already have an Openbase account. Email invites arrive in a later release.
              </DialogDescription>
            </DialogHeader>
            {addError && <ErrorBanner message={addError} />}
            <div>
              <Label htmlFor="member-email">Email</Label>
              <Input
                id="member-email"
                type="email"
                value={addEmail}
                onChange={(e) => setAddEmail(e.target.value)}
                placeholder="teammate@example.com"
                autoComplete="off"
              />
            </div>
            <div>
              <Label htmlFor="member-role">Role</Label>
              <div id="member-role" className="mt-1 flex flex-wrap gap-2">
                <SegmentedOption selected={addRole === "member"} onClick={() => setAddRole("member")}>
                  Member
                </SegmentedOption>
                <SegmentedOption selected={addRole === "admin"} onClick={() => setAddRole("admin")}>
                  Admin
                </SegmentedOption>
                {/* Only an owner may grant owner; the server enforces this too. */}
                {viewerRole === "owner" && (
                  <SegmentedOption selected={addRole === "owner"} onClick={() => setAddRole("owner")}>
                    Owner
                  </SegmentedOption>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAddOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="primary" loading={adding}>
                Add member
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => !open && setRemoveTarget(null)}
        title={me?.id === removeTarget?.user_id ? "Leave this organization?" : "Remove this member?"}
        description={
          me?.id === removeTarget?.user_id
            ? "You will lose access to every project in this organization."
            : `${removeTarget?.email ?? "This person"} will lose access to every project in this organization.`
        }
        confirmLabel={me?.id === removeTarget?.user_id ? "Leave organization" : "Remove member"}
        loading={removing}
        onConfirm={removeMember}
        error={removeError ? <ErrorBanner message={removeError} /> : undefined}
      />

      <ConfirmDialog
        open={transferTarget !== null}
        onOpenChange={(open) => !open && setTransferTarget(null)}
        title="Transfer ownership?"
        description={`${transferTarget?.email ?? "This person"} will become an owner of this organization.`}
        confirmLabel="Transfer ownership"
        loading={transferring}
        onConfirm={transferOwnership}
        error={transferError ? <ErrorBanner message={transferError} /> : undefined}
      >
        {/* Staying a co-owner is a legitimate choice, so demotion is opt-out. */}
        <label className="flex items-center gap-2 text-caption text-foreground">
          <input
            type="checkbox"
            checked={demoteSelf}
            onChange={(e) => setDemoteSelf(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-border"
          />
          Also demote me to admin
        </label>
      </ConfirmDialog>
    </div>
  );
}

function RoleSelect({
  value,
  disabled,
  disabledReason,
  allowOwner,
  onChange,
}: {
  value: OrgRole;
  disabled?: boolean;
  disabledReason?: string;
  allowOwner: boolean;
  onChange: (next: OrgRole) => void;
}) {
  const options: OrgRole[] = allowOwner || value === "owner" ? ["owner", "admin", "member"] : ["admin", "member"];
  const select = (
    <select
      aria-label="Role"
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value as OrgRole)}
      className="h-7 rounded-md border border-input bg-foreground/[0.02] px-1.5 text-caption text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
    >
      {options.map((r) => (
        <option key={r} value={r}>
          {r}
        </option>
      ))}
    </select>
  );
  if (!disabled || !disabledReason) return select;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span title={disabledReason}>{select}</span>
        </TooltipTrigger>
        <TooltipContent>{disabledReason}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function RowMenu({
  member,
  isSelf,
  isLastOwner,
  disabled,
  canTransfer,
  onRemove,
  onTransfer,
}: {
  member: OrgMember;
  isSelf: boolean;
  isLastOwner: boolean;
  disabled?: boolean;
  canTransfer: boolean;
  onRemove: () => void;
  onTransfer: () => void;
}) {
  // The last owner cannot leave: the server would 409, and the fix is to
  // transfer ownership first.
  const blocked = disabled || (isSelf && isLastOwner);
  const reason = disabled
    ? "Only owners can manage owners."
    : "Transfer ownership before leaving.";

  if (blocked) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              aria-disabled="true"
              aria-label={reason}
              title={reason}
              className="inline-flex h-7 w-7 cursor-not-allowed items-center justify-center rounded-md text-muted-foreground/50"
            >
              <MoreHorizontal className="h-4 w-4" aria-hidden />
            </span>
          </TooltipTrigger>
          <TooltipContent>{reason}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={`Actions for ${member.email}`}>
          <MoreHorizontal className="h-4 w-4" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {canTransfer && !isSelf && (
          <DropdownMenuItem onSelect={onTransfer}>Transfer ownership</DropdownMenuItem>
        )}
        <DropdownMenuItem onSelect={onRemove}>
          {isSelf ? "Leave organization" : "Remove from organization"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
