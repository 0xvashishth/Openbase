"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge, Card, EmptyState, Spinner } from "@/components/ui";
import type { Organization } from "@/lib/types";

export function OrgList() {
  const [orgs, setOrgs] = useState<Organization[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const token = authToken();
    if (!token) return;
    api
      .listOrgs(token)
      .then(setOrgs)
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load orgs"));
  }, []);

  if (error) {
    return <p className="text-sm text-red-600">{error}</p>;
  }
  if (!orgs) {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
        <Spinner className="h-4 w-4" /> Loading organizations…
      </div>
    );
  }
  if (orgs.length === 0) {
    return <EmptyState title="No organizations yet" hint="Create your first organization to get started." />;
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {orgs.map((org) => (
        <Link key={org.id} href={`/orgs/${org.id}`} className="group">
          <Card className="p-4 transition-shadow group-hover:shadow-md">
            <div className="flex items-center justify-between">
              <h3 className="font-semibold text-slate-900 group-hover:text-brand-700">{org.name}</h3>
              <Badge tone="blue">{org.slug}</Badge>
            </div>
            <p className="mt-1 text-xs text-slate-500">
              Created {new Date(org.created_at).toLocaleDateString()}
            </p>
          </Card>
        </Link>
      ))}
    </div>
  );
}