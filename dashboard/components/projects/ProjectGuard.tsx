"use client";

import Link from "next/link";
import * as React from "react";
import { Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/feedback";
import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useProject } from "@/lib/project-context";

export function ProjectHeader({ title, subtitle, info }: { title: string; subtitle?: string; info?: string }) {
  const tooltipText = info ?? subtitle;
  return (
    <header className="border-b border-border bg-background px-4 py-4 sm:px-6 sm:py-5">
      <h1 className="flex min-w-0 items-center gap-1.5 truncate text-lg font-semibold tracking-tight text-foreground">
        <span className="truncate">{title}</span>
        {tooltipText && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={`About ${title}`}
                  className="shrink-0 rounded text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <Info className="h-4 w-4" aria-hidden />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right" className="max-w-xs">
                {tooltipText}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </h1>
    </header>
  );
}

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
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <EmptyState title="Project context unavailable" hint="Open this tool from inside a project." />
      </div>
    );
  }

  if (ctx.loading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectOverviewSkeleton />
      </div>
    );
  }

  if (ctx.error) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <EmptyState title={ctx.error} hint="Check the URL or pick another project." />
      </div>
    );
  }

  if (requireConnection && !ctx.hasConnection) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <EmptyState
          title="Connect a database first"
          hint={`${toolName} needs a connected database. Set one up, then come back.`}
          action={
            <Button asChild>
              <Link href={`/orgs/${ctx.orgId}/projects/${ctx.projectId}/connection`}>
                Go to Connection
              </Link>
            </Button>
          }
        />
      </div>
    );
  }

  if (requireTriggers && !ctx.supportsTriggers) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <EmptyState
          title="Native triggers not supported"
          hint="The connected engine does not support native triggers. Only engines that expose native trigger/change capture expose the trigger builder."
        />
      </div>
    );
  }

  if (requireRealtime && !ctx.supportsRealtime) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-6">
        <EmptyState
          title="Native realtime not supported"
          hint="The connected engine does not expose a native change stream. Live WebSocket updates are only available for engines that advertise native realtime support."
        />
      </div>
    );
  }

  return <>{children}</>;
}
