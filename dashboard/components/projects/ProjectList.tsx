"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/feedback";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import type { Project } from "@/lib/types";

export function ProjectList({ orgId }: { orgId: string }) {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api
      .listProjects(token, orgId)
      .then(setProjects)
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load projects"));
  }, [orgId]);

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>;
  if (!projects) {
    return <CardGridSkeleton count={6} label="Loading projects" />;
  }
  if (projects.length === 0) {
    return <EmptyState title="No projects yet" hint="Create a project to start building." />;
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {projects.map((p) => (
        <Link key={p.id} href={`/orgs/${orgId}/projects/${p.id}`} className="group min-w-0">
          <Card className="transition-colors hover:border-foreground/25 hover:shadow-sm">
            <CardContent className="p-4 pt-4 sm:p-4 sm:pt-4">
              <div className="flex items-center justify-between gap-2">
                <h3 className="truncate font-semibold text-foreground">{p.name}</h3>
                <Badge variant="muted">{p.slug}</Badge>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                Created {new Date(p.created_at).toLocaleDateString()}
              </p>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  );
}
