"use client";

import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { ProjectList } from "@/components/projects/ProjectList";
import { CreateProjectForm } from "@/components/projects/CreateProjectForm";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import type { Organization } from "@/lib/types";

export default function OrgDetailPage() {
  const params = useParams<{ orgId: string }>();
  const orgId = params.orgId;
  const [org, setOrg] = useState<Organization | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api
      .getOrg(token, orgId)
      .then(setOrg)
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load org"));
  }, [orgId]);

  return (
    <div>
      <header className="border-b border-border bg-background px-6 py-5">
        {org ? (
          <>
            <div className="flex items-center gap-3">
              <h1 className="text-lg font-semibold tracking-tight text-foreground">{org.name}</h1>
              <Badge variant="secondary">{org.slug}</Badge>
            </div>
            <p className="mt-0.5 text-sm text-muted-foreground">Projects in this organization.</p>
          </>
        ) : error ? (
          <p role="alert" className="text-sm text-destructive">{error}</p>
        ) : (
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
        )}
      </header>

      <div className="mx-auto max-w-5xl px-6 py-6">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm text-muted-foreground">Projects</p>
          <Button onClick={() => setOpen(true)}>
            <Plus className="h-4 w-4" aria-hidden /> New project
          </Button>
        </div>

        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create a project</DialogTitle>
              <DialogDescription>
                Projects hold one database connection plus API keys, functions and triggers.
              </DialogDescription>
            </DialogHeader>
            <CreateProjectForm orgId={orgId} onCreated={() => setOpen(false)} />
          </DialogContent>
        </Dialog>

        <ProjectList orgId={orgId} />
      </div>
    </div>
  );
}
