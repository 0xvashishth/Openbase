"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/feedback";
import { ProjectGuard, ProjectHeader } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";

function SettingsBody() {
  const { project, engine, hasConnection } = useProject();
  if (!project) return <EmptyState title="Project not found" />;
  return (
    <div className="max-w-3xl space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{project.name}</CardTitle>
          <CardDescription>
            slug: {project.slug} · engine: {engine ?? "none"} ·{" "}
            {hasConnection ? "connected" : "not connected"}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href={`/orgs/${project.organization_id}/projects/${project.id}/connection`}>
              Manage connection
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link href={`/orgs/${project.organization_id}/projects/${project.id}/api`}>
              Manage API keys
            </Link>
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Danger zone</CardTitle>
          <CardDescription>Removing the connection destroys provisioned databases.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Use the Connection tab to remove or replace the database. Project deletion via API.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

export default function SettingsPage() {
  useParams<{ orgId: string; projectId: string }>();
  return (
    <div>
      <ProjectHeader title="Settings" subtitle="Project metadata and danger zone." />
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectGuard>
          <SettingsBody />
        </ProjectGuard>
      </div>
    </div>
  );
}
