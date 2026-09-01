"use client";

import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { ProjectList } from "@/components/projects/ProjectList";
import { CreateProjectForm } from "@/components/projects/CreateProjectForm";
import { Button, Spinner } from "@/components/ui";
import type { Organization } from "@/lib/types";

export default function OrgDetailPage() {
  const params = useParams<{ orgId: string }>();
  const orgId = params.orgId;
  const [org, setOrg] = useState<Organization | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

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
      <header className="border-b border-slate-200 bg-white px-6 py-5">
        {org ? (
          <>
            <div className="flex items-center gap-3">
              <h1 className="text-lg font-semibold text-slate-900">{org.name}</h1>
              <span className="rounded-full bg-brand-50 px-2 py-0.5 text-xs font-medium text-brand-700 ring-1 ring-inset ring-brand-600/20">
                {org.slug}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-slate-500">Projects in this organization.</p>
          </>
        ) : error ? (
          <p className="text-sm text-red-600">{error}</p>
        ) : (
          <div className="flex items-center gap-2 text-sm text-slate-400">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        )}
      </header>

      <div className="mx-auto max-w-5xl px-6 py-6">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm text-slate-500">Projects</p>
          <Button onClick={() => setShowCreate((v) => !v)} variant={showCreate ? "secondary" : "primary"}>
            {showCreate ? "Cancel" : "New project"}
          </Button>
        </div>

        {showCreate && (
          <div className="mb-6 max-w-md rounded-xl border border-brand-200 bg-brand-50/40 p-5">
            <h2 className="mb-3 text-sm font-semibold text-slate-800">Create a project</h2>
            <CreateProjectForm orgId={orgId} onCreated={() => setShowCreate(false)} />
          </div>
        )}

        <ProjectList orgId={orgId} />
      </div>
    </div>
  );
}