"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Card, EmptyState, Spinner } from "@/components/ui";
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
      <header className="border-b border-slate-200 bg-white px-6 py-5">
        <h1 className="text-lg font-semibold text-slate-900">Projects</h1>
        <p className="mt-0.5 text-sm text-slate-500">
          {total} project{total === 1 ? "" : "s"} across your organizations.
        </p>
      </header>

      <div className="mx-auto max-w-5xl px-6 py-6">
        {error && <p className="text-sm text-red-600">{error}</p>}
        {!data && (
          <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        )}

        {data && total === 0 && (
          <EmptyState title="No projects yet" hint="Create an organization and add a project to get started." />
        )}

        {data?.map(({ org, projects }) => (
          <section key={org.id} className="mb-6">
            <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">
              {org.name}
            </h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {projects.length === 0 && (
                <p className="text-sm text-slate-400">No projects in this org yet.</p>
              )}
              {projects.map((p) => (
                <Link key={p.id} href={`/orgs/${org.id}/projects/${p.id}`} className="group">
                  <Card className="p-4 transition-shadow group-hover:shadow-md">
                    <h3 className="font-semibold text-slate-900 group-hover:text-brand-700">{p.name}</h3>
                    <p className="mt-1 text-xs text-slate-500">{p.slug}</p>
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