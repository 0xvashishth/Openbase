"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState, PageHeader } from "@/components/ui/feedback";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import type { Organization, Project } from "@/lib/types";

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
    <div>
      <PageHeader
        title="All projects"
        subtitle={
          data
            ? `${total} project${total === 1 ? "" : "s"} across your organizations.`
            : "Loading your projects…"
        }
      />
      <div className="mx-auto max-w-5xl px-6 py-6">
        <div className="mb-4 rounded-md border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
          Tip: pick an organization first — project tools live inside{" "}
          <Link href="/orgs" className="font-medium text-foreground underline underline-offset-4">
            Organizations → Projects
          </Link>
          , Supabase-style. This global view is a search shortcut.
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">{error}</p>
        )}
        {!data && !error && <CardGridSkeleton count={6} label="Loading projects" />}

        {data && total === 0 && (
          <EmptyState title="No projects yet" hint="Create an organization and add a project to get started." />
        )}

        {data?.map(({ org, projects }) => (
          <section key={org.id} className="mb-6">
            <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              {org.name}
            </h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {projects.length === 0 && (
                <p className="text-sm text-muted-foreground">No projects in this org yet.</p>
              )}
              {projects.map((p) => (
                <Link key={p.id} href={`/orgs/${org.id}/projects/${p.id}`} className="group">
                  <Card className="transition-shadow group-hover:shadow-md">
                    <CardContent className="p-4 pt-4">
                      <h3 className="font-semibold text-foreground">{p.name}</h3>
                      <p className="mt-1 text-xs text-muted-foreground">{p.slug}</p>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
