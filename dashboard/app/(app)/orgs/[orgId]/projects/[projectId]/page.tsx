"use client";

import Link from "next/link";
import { Database, KeyRound, Network, Plug, Radio, SquareTerminal, Table2, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { buttonVariants } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/feedback";
import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";
import { ProjectGuard, ProjectHeader } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";

function OverviewBody() {
  const { orgId, projectId, project, engine, hasConnection, loading, error } = useProject();

  if (loading) {
    return <ProjectOverviewSkeleton />;
  }
  if (error || !project) {
    return <EmptyState title={error ?? "Project not found"} />;
  }

  const base = `/orgs/${orgId}/projects/${projectId}`;
  const tools = [
    { href: `${base}/tables`, icon: Table2, title: "Tables", desc: "Browse collections and rows.", locked: !hasConnection },
    { href: `${base}/schema`, icon: Network, title: "Schema", desc: "Visual tables and relationships.", locked: !hasConnection },
    { href: `${base}/sql`, icon: SquareTerminal, title: "SQL Editor", desc: "Run queries with highlighting.", locked: !hasConnection },
    { href: `${base}/api`, icon: KeyRound, title: "API Keys", desc: "Auto-generated REST API access.", locked: false },
    { href: `${base}/functions`, icon: Zap, title: "Functions", desc: "Serverless event handlers.", locked: !hasConnection },
    { href: `${base}/triggers`, icon: Database, title: "Triggers", desc: "Data events → functions/webhooks.", locked: !hasConnection },
    { href: `${base}/realtime`, icon: Radio, title: "Realtime", desc: "Live WebSocket change streams.", locked: !hasConnection },
  ];

  return (
    <div className="mx-auto w-full max-w-4xl space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle>{project.name}</CardTitle>
            <Badge variant="secondary">{project.slug}</Badge>
            {engine && <Badge variant="secondary">{engine}</Badge>}
            <Badge variant={hasConnection ? "success" : "warning"}>
              {hasConnection ? "connected" : "not connected"}
            </Badge>
          </div>
          <CardDescription>
            {hasConnection
              ? "Database connected. Pick a tool below."
              : "This project needs a database. Connect one, then browse tables."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!hasConnection ? (
            <Link href={`${base}/connection`} className={buttonVariants()}>
              <Plug className="h-4 w-4" aria-hidden /> Connect a database
            </Link>
          ) : (
            <Link href={`${base}/tables`} className={buttonVariants()}>
              <Table2 className="h-4 w-4" aria-hidden /> Browse tables
            </Link>
          )}
        </CardContent>
      </Card>

      <div className="grid items-stretch gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {tools.map((t) => (
          <Link key={t.title} href={t.href} aria-disabled={t.locked || undefined} className="block h-full min-w-0">
            <Card className="flex h-full flex-col transition-colors hover:border-foreground/25 hover:shadow-sm">
              <CardContent className="flex flex-1 flex-col justify-center p-4 pt-4 sm:p-4 sm:pt-4">
                <div className="flex items-center gap-2">
                  <t.icon className="h-4 w-4 text-muted-foreground" aria-hidden />
                  <p className="text-sm font-semibold text-foreground">{t.title}</p>
                  {t.locked && <Badge variant="muted">locked</Badge>}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">{t.desc}</p>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}

export default function ProjectOverviewPage() {
  return (
    <div>
      <ProjectHeader title="Overview" subtitle="Project status and shortcuts." />
      <div className="mx-auto w-full max-w-6xl px-6 py-6">
        <ProjectGuard>
          <OverviewBody />
        </ProjectGuard>
      </div>
    </div>
  );
}
