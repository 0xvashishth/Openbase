"use client";

import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { TableBrowser } from "@/components/data/TableBrowser";
import { EmptyState, Spinner } from "@/components/ui";

export default function ProjectDataTab() {
  const params = useParams<{ orgId: string; projectId: string }>();
  const { orgId, projectId } = params;
  const [connected, setConnected] = useState<boolean | null>(null);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api
      .getConnection(token, projectId)
      .then((c) => setConnected(c.status === "connected"))
      .catch(() => setConnected(false));
  }, [projectId]);

  return (
    <div>
      <header className="border-b border-slate-200 bg-white px-6 py-5">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-slate-900">Tables</h1>
          <Link
            href={`/orgs/${orgId}/projects/${projectId}`}
            className="text-sm text-brand-600 hover:text-brand-700"
          >
            ← Overview
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-6xl px-6 py-6">
        {connected === null ? (
          <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
            <Spinner className="h-4 w-4" /> Loading…
          </div>
        ) : connected ? (
          <TableBrowser projectId={projectId} />
        ) : (
          <EmptyState
            title="Connect a database first"
            hint="Attach a database on the Connection tab, then browse tables here."
          />
        )}
      </div>
    </div>
  );
}