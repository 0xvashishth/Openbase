"use client";

import Link from "next/link";
import * as React from "react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/feedback";
import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";
import { useProject } from "@/lib/project-context";

/**
 * Gates tool content on connection + capability state from the single
 * ProjectProvider (provided by AppShell — no extra fetching here).
 */
export function ProjectGuard({
  children,
  requireConnection = false,
  requireTriggers = false,
  requireRealtime = false,
  toolName = "This tool",
}: {
  children: React.ReactNode;
  requireConnection?: boolean;
  requireTriggers?: boolean;
  requireRealtime?: boolean;
  toolName?: string;
}) {
  let ctx;
  try {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    ctx = useProject();
  } catch {
    return <EmptyState title="Project context unavailable" hint="Open this tool from inside a project." />;
  }

  if (ctx.loading) {
    return <ProjectOverviewSkeleton />;
  }

  if (ctx.error) {
    return <EmptyState title={ctx.error} hint="Check the URL or pick another project." />;
  }

  if (requireConnection && !ctx.hasConnection) {
    return (
      <EmptyState
        title="Connect a database first"
        hint={`${toolName} needs a connected database. Set one up, then come back.`}
        action={
          <Button asChild>
            <Link href={`/orgs/${ctx.orgId}/projects/${ctx.projectId}/db-source`}>
              Go to DB Source
            </Link>
          </Button>
        }
      />
    );
  }

  if (requireTriggers && !ctx.supportsTriggers) {
    return (
      <EmptyState
        title="Native triggers not supported"
        hint="The connected engine does not support native triggers. Only engines that expose native trigger/change capture expose the trigger builder."
      />
    );
  }

  if (requireRealtime && !ctx.supportsRealtime) {
    return (
      <EmptyState
        title="Native realtime not supported"
        hint="The connected engine does not expose a native change stream. Live WebSocket updates are only available for engines that advertise native realtime support."
      />
    );
  }

  return <>{children}</>;
}
