"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/feedback";
import { CardGridSkeleton } from "@/components/ui/skeletons";
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
    return (
      <p role="alert" className="text-caption text-destructive">{error}</p>
    );
  }
  if (!orgs) {
    return <CardGridSkeleton count={6} label="Loading organizations" />;
  }
  if (orgs.length === 0) {
    return <EmptyState title="No organizations yet" hint="Create your first organization to get started." />;
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {orgs.map((org) => (
        <Link key={org.id} href={`/orgs/${org.id}`} className="group min-w-0">
          <Card className="transition-colors hover:border-foreground/25">
            <CardContent className="p-4 pt-4 sm:p-4 sm:pt-4">
              <div className="flex items-center justify-between gap-2">
                <h3 className="truncate text-caption font-w510 text-foreground-strong">{org.name}</h3>
                <Badge variant="secondary">{org.slug}</Badge>
              </div>
              <p className="mt-1.5 text-label text-muted-foreground">
                Created {new Date(org.created_at).toLocaleDateString()}
              </p>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  );
}
