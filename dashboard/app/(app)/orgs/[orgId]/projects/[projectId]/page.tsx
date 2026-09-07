"use client";

import Link from "next/link";
import { Cable, Database, KeyRound, Network, Plug, Radio, SquareTerminal, Table2, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { buttonVariants } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/feedback";
import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";
import { PageShell } from "@/components/layout/PageShell";
import { PROJECT_TOOL_META, projectToolPath } from "@/components/layout/nav";

// Tiles shown on the overview grid (overview + settings live elsewhere).
// Slugs/labels/descriptions derive from nav.ts; icons + connection locks
// stay local because they are presentation.
const TILE_ICONS: Record<string, typeof Table2> = {
  tables: Table2,
  schema: Network,
  sql: SquareTerminal,
  connect: Cable,
  api: KeyRound,
  functions: Zap,
  triggers: Database,
  realtime: Radio,
  "db-source": Plug,
};

const TILE_SLUGS = ["tables", "schema", "sql", "connect", "api", "functions", "triggers", "realtime", "db-source"];

function OverviewBody() {
  const { orgId, projectId, project, engine, hasConnection, loading, error } = useProject();

  if (loading) {
    return <ProjectOverviewSkeleton />;
  }
  if (error || !project) {
    return <EmptyState title={error ?? "Project not found"} />;
  }

  const base = `/orgs/${orgId}/projects/${projectId}`;
  const tools = PROJECT_TOOL_META.filter((t) => TILE_SLUGS.includes(t.slug)).map((t) => {
    const Icon = TILE_ICONS[t.slug] ?? Table2;
    const unlocked = t.slug === "connect" || t.slug === "api" || t.slug === "db-source";
    return {
      href: projectToolPath(orgId, projectId, t.slug),
      icon: Icon,
      title: t.label,
      desc: t.description,
      locked: !unlocked && !hasConnection,
    };
  });

  return (
    <div className="space-y-4">
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
              ? "Database attached. Browse it here, or wire an app to it on the Connect tab."
              : "This project needs a database. Attach one on DB Source, then browse tables."}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {!hasConnection ? (
            <Link href={`${base}/db-source`} className={buttonVariants()}>
              <Plug className="h-4 w-4" aria-hidden /> Attach a database
            </Link>
          ) : (
            <>
              <Link href={`${base}/tables`} className={buttonVariants()}>
                <Table2 className="h-4 w-4" aria-hidden /> Browse tables
              </Link>
              <Link href={`${base}/connect`} className={buttonVariants({ variant: "outline" })}>
                <Cable className="h-4 w-4" aria-hidden /> Connect your app
              </Link>
            </>
          )}
        </CardContent>
      </Card>

      <div className="grid items-stretch gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {tools.map((t) => (
          <Link key={t.title} href={t.href} aria-disabled={t.locked || undefined} className="block h-full min-w-0">
            <Card className="flex h-full flex-col transition-colors hover:border-foreground/25">
              <CardContent className="flex flex-1 flex-col justify-center p-4 pt-4 sm:p-4 sm:pt-4">
                <div className="flex items-center gap-2">
                  <t.icon className="h-4 w-4 text-muted-foreground" aria-hidden />
                  <p className="text-caption font-w510 text-foreground-strong">{t.title}</p>
                  {t.locked && <Badge variant="muted">locked</Badge>}
                </div>
                <p className="mt-1.5 text-label text-muted-foreground">{t.desc}</p>
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
    <PageShell>
      <ProjectGuard>
        <OverviewBody />
      </ProjectGuard>
    </PageShell>
  );
}
