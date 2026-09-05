"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { useOrg } from "@/lib/org-context";
import { useProject } from "@/lib/project-context";
import { useToast } from "@/components/ui/toast";
import { Badge, StatusBadge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CopyField } from "@/components/ui/code-block";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ErrorBanner } from "@/components/ui/feedback";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FormSkeleton } from "@/components/ui/skeletons";

const SLUG_RE = /^[a-z0-9][a-z0-9-]{1,38}$/;

export function ProjectSettingsPanel() {
  const { orgId, projectId, project, connection, engine, hasConnection, loading, refresh } = useProject();
  const { can } = useOrg();
  const router = useRouter();
  const toast = useToast();

  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [slugError, setSlugError] = useState<string | null>(null);

  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Counts for the delete confirmation, so the dialog can enumerate exactly
  // what is about to be destroyed rather than saying "and related data".
  const [counts, setCounts] = useState<{ keys: number; triggers: number; functions: number } | null>(null);

  const canEdit = can("rename_project");
  const canDelete = can("delete_project");
  const isProvisioned = connection?.mode === "provisioned";

  useEffect(() => {
    if (!project) return;
    setName(project.name);
    setSlug(project.slug);
  }, [project]);

  useEffect(() => {
    const token = authToken();
    if (!token || !projectId) return;
    let cancelled = false;
    Promise.all([
      api.listAPIKeys(token, projectId).catch(() => []),
      api.listTriggers(token, projectId).catch(() => []),
      api.listFunctions(token, projectId).catch(() => []),
    ]).then(([keys, triggers, functions]) => {
      if (cancelled) return;
      setCounts({
        keys: (keys ?? []).filter((k) => !k.revoked_at).length,
        triggers: (triggers ?? []).length,
        functions: (functions ?? []).length,
      });
    });
    return () => {
      cancelled = true;
    };
  }, [projectId]);

  const dirty = useMemo(
    () => Boolean(project) && (name.trim() !== project!.name || slug.trim() !== project!.slug),
    [project, name, slug]
  );

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!project) return;
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
      await api.updateProject(
        token,
        projectId,
        nextName === project.name ? undefined : nextName,
        nextSlug === project.slug ? undefined : nextSlug
      );
      refresh();
      toast.success("Project updated");
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setSlugError("Another project in this organization already uses that slug.");
      } else {
        setSaveError(err instanceof Error ? err.message : "Failed to save changes");
      }
    } finally {
      setSaving(false);
    }
  }

  async function destroy() {
    if (!project) return;
    setDeleteError(null);
    setDeleting(true);
    try {
      const token = authToken();
      if (!token) throw new Error("Not authenticated");
      await api.deleteProject(token, projectId);
      toast.success("Project deleted", project.name);
      router.replace(`/orgs/${orgId}`);
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Failed to delete project");
    } finally {
      setDeleting(false);
    }
  }

  if (loading && !project) return <FormSkeleton label="Loading project settings" />;
  if (!project) return <ErrorBanner message="Project not found" />;

  const base = `/orgs/${orgId}/projects/${projectId}`;

  return (
    <div className="max-w-3xl space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
          <CardDescription>
            {canEdit
              ? "Rename the project or change its display identifier."
              : "Only owners and admins can change these settings."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={save} className="space-y-4">
            {saveError && <ErrorBanner message={saveError} />}
            <div>
              <Label htmlFor="project-name">Name</Label>
              <Input
                id="project-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                readOnly={!canEdit}
              />
            </div>
            <div>
              <Label htmlFor="project-slug">Slug</Label>
              <Input
                id="project-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                readOnly={!canEdit}
                aria-invalid={slugError ? true : undefined}
                aria-describedby={slugError ? "project-slug-error" : "project-slug-hint"}
                className="font-mono"
              />
              {slugError ? (
                <p id="project-slug-error" role="alert" className="mt-1.5 text-label text-destructive">
                  {slugError}
                </p>
              ) : (
                <p id="project-slug-hint" className="mt-1.5 text-label text-muted-foreground">
                  Unique within this organization. The dashboard routes by id, so changing it breaks no links.
                </p>
              )}
            </div>
            <div className="flex items-center gap-3">
              <Button type="submit" variant="primary" loading={saving} disabled={!canEdit || !dirty}>
                Save changes
              </Button>
              {!canEdit && (
                <p className="text-label text-muted-foreground">Requires the admin or owner role.</p>
              )}
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
          <CardDescription>Read-only facts about this project.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <CopyField label="Project ID" value={project.id} />
          <dl className="grid gap-x-6 gap-y-2 text-caption sm:grid-cols-2">
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Engine</dt>
              <dd className="mt-0.5">
                {engine ? <Badge>{engine}</Badge> : <span className="text-muted-foreground">none</span>}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Database</dt>
              <dd className="mt-0.5">
                <StatusBadge tone={hasConnection ? "success" : "muted"}>
                  {hasConnection ? "connected" : "not connected"}
                </StatusBadge>
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Mode</dt>
              <dd className="mt-0.5">
                {connection ? (
                  <Badge variant="secondary">{connection.mode === "provisioned" ? "Provisioned" : "BYODB"}</Badge>
                ) : (
                  <span className="text-muted-foreground">—</span>
                )}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-2 sm:block">
              <dt className="text-muted-foreground">Created</dt>
              <dd className="mt-0.5 text-foreground">
                {new Date(project.created_at).toLocaleDateString()}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Quick links</CardTitle>
          <CardDescription>Jump to the tools that manage this project's data and access.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href={`${base}/db-source`}>Manage DB source</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href={`${base}/connect`}>Connect an app</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href={`${base}/api`}>Manage API keys</Link>
          </Button>
        </CardContent>
      </Card>

      <Card className="border-destructive/30">
        <CardHeader>
          <CardTitle>Danger zone</CardTitle>
          <CardDescription>These actions destroy data.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="min-w-0">
              <p className="text-caption font-w510 text-foreground-strong">Remove database</p>
              <p className="text-label text-muted-foreground">
                Detach or replace the database from the DB Source tab.
              </p>
            </div>
            <Button asChild variant="outline">
              <Link href={`${base}/db-source`}>Go to DB Source</Link>
            </Button>
          </div>

          {canDelete && (
            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
              <div className="min-w-0">
                <p className="text-caption font-w510 text-foreground-strong">Delete project</p>
                <p className="text-label text-muted-foreground">
                  Deletes the project and everything scoped to it. Cannot be undone.
                </p>
              </div>
              <Button
                variant="destructive"
                onClick={() => {
                  setDeleteError(null);
                  setConfirmOpen(true);
                }}
              >
                Delete project
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete this project?"
        confirmLabel="Delete project"
        confirmPhrase={project.slug}
        loading={deleting}
        onConfirm={destroy}
        error={deleteError ? <ErrorBanner message={deleteError} /> : undefined}
      >
        <div className="space-y-2 text-caption">
          <p className="text-foreground">This permanently destroys:</p>
          <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
            {isProvisioned && (
              /* PHASES 18.1: provisioned containers have no persistent volumes
                 yet, so this really is unrecoverable. Say so. */
              <li className="text-destructive">
                The provisioned {engine ?? "database"} container and all data in it. This is not
                recoverable — there is no backup.
              </li>
            )}
            {!isProvisioned && connection && (
              <li>The saved connection to your database. Your database itself is left untouched.</li>
            )}
            <li>
              {counts ? counts.keys : "All"} API key{counts?.keys === 1 ? "" : "s"} — apps using them stop
              working immediately
            </li>
            <li>
              {counts ? counts.triggers : "All"} trigger{counts?.triggers === 1 ? "" : "s"}
            </li>
            <li>
              {counts ? counts.functions : "All"} function{counts?.functions === 1 ? "" : "s"}
            </li>
          </ul>
        </div>
      </ConfirmDialog>
    </div>
  );
}
