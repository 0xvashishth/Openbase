"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge, Card, EmptyState, Spinner } from "@/components/ui";
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

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!projects) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading projects…
      </div>
    );
  }
  if (projects.length === 0) {
    return <EmptyState title="No projects yet" hint="Create a project to start building." />;
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {projects.map((p) => (
        <Link key={p.id} href={`/orgs/${orgId}/projects/${p.id}`} className="group">
          <Card className="p-4 transition-shadow group-hover:shadow-md">
            <div className="flex items-center justify-between">
              <h3 className="font-semibold text-slate-900 group-hover:text-brand-700">{p.name}</h3>
              <Badge tone="slate">{p.slug}</Badge>
            </div>
            <p className="mt-1 text-xs text-slate-500">
              Created {new Date(p.created_at).toLocaleDateString()}
            </p>
          </Card>
        </Link>
      ))}
    </div>
  );
}