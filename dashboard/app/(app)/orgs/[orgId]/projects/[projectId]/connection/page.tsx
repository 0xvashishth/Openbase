"use client";

import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { ConnectionPanel } from "@/components/projects/ConnectionPanel";
import { Spinner } from "@/components/ui";
import type { Project } from "@/lib/types";

export default function ProjectConnectionTab() {
  const params = useParams<{ orgId: string; projectId: string }>();
  const { orgId, projectId } = params;
  const [project, setProject] = useState<Project | null>(null);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api.listProjects(token, orgId).then((projects) => {
      setProject(projects.find((p) => p.id === projectId) ?? null);
    });
  }, [orgId, projectId]);

  return (
    <div>
      <header className="border-b border-slate-200 bg-white px-6 py-5">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-slate-900">
            {project ? project.name : "Project"}
          </h1>
          <Link
            href={`/orgs/${orgId}/projects/${projectId}`}
            className="text-sm text-brand-600 hover:text-brand-700"
          >
            ← Overview
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-5xl px-6 py-6">
        {!project ? (
          <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        ) : (
          <ConnectionPanel projectId={projectId} />
        )}
      </div>
    </div>
  );
}