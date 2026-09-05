"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/feedback";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import type { Organization, Project } from "@/lib/types";
import { PageShell } from "@/components/layout/PageShell";

interface OrgWithProjects {
  org: Organization;
  projects: Project[];
}

export default function ProjectsPage() {
  const [data, setData] = useState<OrgWithProjects[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    (async () => {
      try {
        const orgs = await api.listOrgs(token);
        const withProjects = await Promise.all(
          orgs.map(async (org) => ({
            org,
            projects: await api.listProjects(token, org.id),
          }))
        );
        setData(withProjects);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load projects");
      }
    })();
  }, []);

  const allProjects = data?.flatMap((d) => d.projects) ?? [];
  const total = allProjects.length;

  return (
    <PageShell>
        {error && (
          <p role="alert" className="text-caption text-destructive">{error}</p>
        )}
        {!data && !error && <CardGridSkeleton count={6} label="Loading projects" />}

        {data && total === 0 && (
          <EmptyState title="No projects yet" hint="Create an organization and add a project to get started." />
        )}

        {data?.map(({ org, projects }) => (
          <section key={org.id} className="mb-6">
            <h2 className="mb-2 text-micro font-w510 uppercase tracking-wide text-muted-foreground">
              {org.name}
            </h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {projects.length === 0 && (
                <p className="text-caption text-muted-foreground">No projects in this org yet.</p>
              )}
              {projects.map((p) => (
                <Link key={p.id} href={`/orgs/${org.id}/projects/${p.id}`} className="group min-w-0">
                  <Card className="transition-colors hover:border-foreground/25">
                    <CardContent className="p-4 pt-4 sm:p-4 sm:pt-4">
                      <h3 className="truncate text-caption font-w510 text-foreground-strong">{p.name}</h3>
                      <p className="mt-1.5 truncate font-mono text-label text-muted-foreground">{p.slug}</p>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          </section>
        ))}
    </PageShell>
  );
}
