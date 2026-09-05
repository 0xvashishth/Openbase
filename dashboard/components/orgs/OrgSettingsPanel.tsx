"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { useOrg } from "@/lib/org-context";
import { useToast } from "@/components/ui/toast";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CopyField } from "@/components/ui/code-block";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ErrorBanner } from "@/components/ui/feedback";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FormSkeleton } from "@/components/ui/skeletons";
import type { Project } from "@/lib/types";

/** Mirrors the server's slug rule so a rejection is caught before the request. */
const SLUG_RE = /^[a-z0-9][a-z0-9-]{1,38}$/;

export function OrgSettingsPanel() {
  const { orgId, org, role, can, loading, error, refresh } = useOrg();
  const router = useRouter();
  const toast = useToast();

  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [slugError, setSlugError] = useState<string | null>(null);

  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [blockingProjects, setBlockingProjects] = useState<Project[] | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const [projectCount, setProjectCount] = useState<number | null>(null);
  const [memberCount, setMemberCount] = useState<number | null>(null);

  const canEdit = can("rename_org");
  const canDelete = can("delete_org");

  // Seed the form from the loaded org; re-seed on refresh so a save that
  // normalises input (trimming) is reflected.
  useEffect(() => {
    if (!org) return;
    setName(org.name);
    setSlug(org.slug);
  }, [org]);

  // Counts for the Details card. Best-effort: a failure here must not block
  // renaming the org.
  useEffect(() => {
    const token = authToken();
    if (!token || !orgId) return;
    let cancelled = false;
    api
      .listProjects(token, orgId)
      .then((ps) => !cancelled && setProjectCount(ps?.length ?? 0))
      .catch(() => !cancelled && setProjectCount(null));
    api
      .listMembers(token, orgId)
      .then((ms) => !cancelled && setMemberCount(ms?.length ?? 0))
      .catch(() => !cancelled && setMemberCount(null));
    return () => {
      cancelled = true;
    };
  }, [orgId]);

  const dirty = useMemo(
    () => Boolean(org) && (name.trim() !== org!.name || slug.trim() !== org!.slug),
    [org, name, slug]
  );

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!org) return;
    setSaveError(null);
    setSlugError(null);

    const nextName = name.trim();
    const nextSlug = slug.trim();
    if (!nextName) {
      setSaveError("Name cannot be empty.");
      return;
    }
    if (!SLUG_RE.test(nextSlug)) {
      setSlugError("Use 2–39 lowercase letters, numbers or hyphens, starting with a letter or number.");
      return;
    }

    setSaving(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.updateOrg(
        token,
        orgId,
        nextName === org.name ? undefined : nextName,
        nextSlug === org.slug ? undefined : nextSlug
      );
      refresh();
      toast.success("Organization updated");
    } catch (err) {
      // A slug collision belongs on the slug field, not in a page-level banner.
      if (err instanceof ApiError && err.status === 409) {
        setSlugError("That slug is already in use.");
      } else {
        setSaveError(err instanceof Error ? err.message : "Failed to save changes");
      }
    } finally {
      setSaving(false);
    }
  }

  async function destroy() {
    if (!org) return;
    setDeleteError(null);
    setBlockingProjects(null);
    setDeleting(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteOrg(token, orgId, org.slug);
      toast.success("Organization deleted");
      router.replace("/orgs");
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // The org still has projects. Show exactly which ones, each linking to
        // its own settings page, instead of a dead-end "conflict" message.
        const token = authToken();
        try {
          const ps = token ? await api.listProjects(token, orgId) : [];
          setBlockingProjects(ps ?? []);
        } catch {
          setBlockingProjects([]);
        }
        setDeleteError(null);
      } else {
        setDeleteError(err instanceof Error ? err.message : "Failed to delete organization");
      }
    } finally {
      setDeleting(false);
    }
  }

  if (loading && !org) return <FormSkeleton label="Loading organization settings" />;
  if (error) return <ErrorBanner message={error} />;
  if (!org) return <ErrorBanner message="Organization not found" />;

  return (
    <div className="max-w-3xl space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>
            {canEdit
              ? "Rename the organization or change its display identifier."
              : "Only owners and admins can change these settings."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={save} className="space-y-4">
            {saveError && <ErrorBanner message={saveError} />}
            <div>
              <Label htmlFor="org-name">Name</Label>
              <Input
                id="org-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                readOnly={!canEdit}
                aria-describedby={canEdit ? undefined : "org-readonly-hint"}
              />
            </div>
            <div>
              <Label htmlFor="org-slug">Slug</Label>
              <Input
                id="org-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                readOnly={!canEdit}
                aria-invalid={slugError ? true : undefined}
                aria-describedby={slugError ? "org-slug-error" : "org-slug-hint"}
                className="font-mono"
              />
              {slugError ? (
                <p id="org-slug-error" role="alert" className="mt-1.5 text-label text-destructive">
                  {slugError}
                </p>
              ) : (
                <p id="org-slug-hint" className="mt-1.5 text-label text-muted-foreground">
                  A display identifier. The dashboard routes by id, so changing this breaks no links.
                </p>
              )}
            </div>
            <div className="flex items-center gap-3">
              <Button type="submit" variant="primary" loading={saving} disabled={!canEdit || !dirty}>
                Save changes
              </Button>
              {!canEdit && (
                <p id="org-readonly-hint" className="text-label text-muted-foreground">
                  Requires the admin or owner role.
                </p>
              )}
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
          <CardDescription>Read-only facts about this organization.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <CopyField label="Organization ID" value={org.id} />
          <dl className="grid gap-x-6 gap-y-2 text-caption sm:grid-cols-2">
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Your role</dt>
              <dd className="mt-0.5">
                <Badge variant="secondary">{role}</Badge>
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Created</dt>
              <dd className="mt-0.5 text-foreground">
                {new Date(org.created_at).toLocaleDateString()}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Members</dt>
              <dd className="mt-0.5">
                <Link
                  href={`/orgs/${orgId}/members`}
                  className="text-foreground underline-offset-4 hover:underline"
                >
                  {memberCount === null ? "View members" : `${memberCount} member${memberCount === 1 ? "" : "s"}`}
                </Link>
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Projects</dt>
              <dd className="mt-0.5">
                <Link href={`/orgs/${orgId}`} className="text-foreground underline-offset-4 hover:underline">
                  {projectCount === null
                    ? "View projects"
                    : `${projectCount} project${projectCount === 1 ? "" : "s"}`}
                </Link>
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      {/* Hidden, not disabled, for non-owners: a control they can never use is
          noise, and the server would 403 anyway. */}
      {canDelete && (
        <Card className="border-destructive/30">
          <CardHeader>
            <CardTitle>Danger zone</CardTitle>
            <CardDescription>
              Deleting an organization removes its memberships permanently. Projects must be deleted first.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              variant="destructive"
              onClick={() => {
                setBlockingProjects(null);
                setDeleteError(null);
                setConfirmOpen(true);
              }}
            >
              Delete organization
            </Button>
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete this organization?"
        description={
          blockingProjects && blockingProjects.length > 0
            ? undefined
            : "This removes the organization and every membership in it. It cannot be undone."
        }
        confirmLabel="Delete organization"
        confirmPhrase={org.slug}
        loading={deleting}
        onConfirm={destroy}
        error={deleteError ? <ErrorBanner message={deleteError} /> : undefined}
      >
        {blockingProjects && blockingProjects.length > 0 && (
          <div className="space-y-2">
            <p className="text-caption text-foreground">Delete these projects first.</p>
            <ul className="space-y-1">
              {blockingProjects.map((p) => (
                <li key={p.id}>
                  <Link
                    href={`/orgs/${orgId}/projects/${p.id}/settings`}
                    className="text-caption text-foreground underline-offset-4 hover:underline"
                  >
                    {p.name}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )}
      </ConfirmDialog>
    </div>
  );
}
